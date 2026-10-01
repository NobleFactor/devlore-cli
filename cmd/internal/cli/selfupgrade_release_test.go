// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// fakeGitHub serves a repository's releases as GitHub's API does, from one host, and their assets from another, as
// GitHub serves `browser_download_url` from github.com and not from api.github.com.
type fakeGitHub struct {
	api      *httptest.Server
	assets   *httptest.Server
	mu       sync.Mutex
	releases []githubRelease          // newest first, as GitHub lists them
	files    map[string][]byte        // each asset's content, by name
	stall    string                   // an asset whose download stops halfway and waits for the client to go
	stalled  chan struct{}            // closed once the stalled download has sent its first half
	requests map[string]int           // API requests, by path and query
	apiAuth  []string                 // the Authorization header of each API request
	fileAuth []string                 // the Authorization header of each asset request
	answers  map[string]*cannedAnswer // a canned answer in place of the release, by API path
}

// cannedAnswer is a canned API response: a status, headers, and GitHub's error body.
type cannedAnswer struct {
	status  int
	headers map[string]string
	message string
}

// newFakeGitHub starts the two hosts and stops them when the test ends.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `*fakeGitHub`: the fake, with no releases yet.
func newFakeGitHub(t *testing.T) *fakeGitHub {

	t.Helper()

	fake := &fakeGitHub{
		files:    map[string][]byte{},
		stalled:  make(chan struct{}),
		requests: map[string]int{},
		answers:  map[string]*cannedAnswer{},
	}
	fake.api = httptest.NewServer(http.HandlerFunc(fake.serveAPI))
	fake.assets = httptest.NewServer(http.HandlerFunc(fake.serveAsset))
	t.Cleanup(fake.api.Close)
	t.Cleanup(fake.assets.Close)

	return fake
}

// environment returns the upgrade's environment pointed at this fake, for this platform, with no children to run.
//
// Parameters:
//   - `token`: GH_TOKEN; "" asks anonymously.
//
// Returns:
//   - `upgradeEnvironment`: the environment.
func (f *fakeGitHub) environment(token string) upgradeEnvironment {

	return upgradeEnvironment{
		apiBase: f.api.URL + "/repos/NobleFactor/devlore-cli",
		client:  f.api.Client(),
		token:   token,
		goos:    runtime.GOOS,
		goarch:  runtime.GOARCH,
	}
}

// publish adds releases, newest first, after those already published.
//
// Parameters:
//   - `releases`: the releases, newest first.
func (f *fakeGitHub) publish(releases ...githubRelease) {

	f.mu.Lock()
	defer f.mu.Unlock()

	f.releases = append(f.releases, releases...)
}

// serve makes the asset host serve `content` as the asset `name`, in place of anything it served under that name.
//
// Parameters:
//   - `name`: the asset's name.
//   - `content`: what it serves.
func (f *fakeGitHub) serve(name string, content []byte) {

	f.mu.Lock()
	defer f.mu.Unlock()

	f.files[name] = content
}

// stallOn makes the asset host stop halfway through the asset `name` and wait for the client to go away.
//
// Parameters:
//   - `name`: the asset's name.
func (f *fakeGitHub) stallOn(name string) {

	f.mu.Lock()
	defer f.mu.Unlock()

	f.stall = name
}

// release returns a published release carrying this platform's archive and the checksums file.
//
// The archive holds the programs, built by [packRelease]; the checksums file lists it; GitHub's digest is its SHA-256.
//
// Parameters:
//   - `t`: the test harness.
//   - `tag`: the release's tag.
//   - `ref`: the ref its body names, as release.yaml writes it.
//   - `programs`: the programs its archive carries.
//
// Returns:
//   - `githubRelease`: the release, not yet published.
func (f *fakeGitHub) release(t *testing.T, tag, ref string, programs ...string) githubRelease {

	t.Helper()

	name, content := packRelease(t, tag, programs...)
	checksums := checksumsFor(map[string][]byte{name: content})

	f.mu.Lock()
	defer f.mu.Unlock()

	f.files[name] = content
	f.files[checksumsName(tag)] = checksums

	return githubRelease{
		TagName:    tag,
		Prerelease: ref == developRef,
		Body:       fmt.Sprintf("Release %s\n\nSource: NobleFactor/devlore-cli@0123abc\nRef: %s", tag, ref),
		Assets: []githubAsset{
			{Name: checksumsName(tag), BrowserDownloadURL: f.assetURL(tag, checksumsName(tag)),
				Digest: "sha256:" + sha256Of(checksums)},
			{Name: name, BrowserDownloadURL: f.assetURL(tag, name), Digest: "sha256:" + sha256Of(content)},
		},
	}
}

// bare returns a release with no assets: enough to be found, not to be installed.
//
// Parameters:
//   - `tag`: the release's tag.
//   - `ref`: the ref its body names.
//
// Returns:
//   - `githubRelease`: the release, not yet published.
func bare(tag, ref string) githubRelease {
	return githubRelease{TagName: tag, Prerelease: ref == developRef, Body: "Ref: " + ref}
}

// bareReleases returns `count` asset-less releases on one ref, newest first, their tags numbered from `first`.
//
// Parameters:
//   - `count`: how many.
//   - `first`: the number in the newest one's tag.
//   - `ref`: the ref every body names.
//
// Returns:
//   - `[]githubRelease`: the releases.
func bareReleases(count, first int, ref string) []githubRelease {

	releases := make([]githubRelease, 0, count)
	for i := range count {
		releases = append(releases, bare(fmt.Sprintf("v0.0.%d", first-i), ref))
	}

	return releases
}

// assetURL returns where the asset host serves an asset.
//
// Parameters:
//   - `tag`: the release's tag.
//   - `name`: the asset's name.
//
// Returns:
//   - `string`: the asset's `browser_download_url`.
func (f *fakeGitHub) assetURL(tag, name string) string {
	return f.assets.URL + "/NobleFactor/devlore-cli/releases/download/" + tag + "/" + name
}

// apiRequests returns how many API requests asked for `pathAndQuery`, or for anything when it is empty.
//
// Parameters:
//   - `pathAndQuery`: the request's path below the repository, and its query; "" counts every request.
//
// Returns:
//   - `int`: the count.
func (f *fakeGitHub) apiRequests(pathAndQuery string) int {

	f.mu.Lock()
	defer f.mu.Unlock()

	if pathAndQuery != "" {
		return f.requests[pathAndQuery]
	}

	total := 0
	for _, count := range f.requests {
		total += count
	}

	return total
}

// assetRequests returns how many asset requests the asset host served.
//
// Returns:
//   - `int`: the count.
func (f *fakeGitHub) assetRequests() int {

	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.fileAuth)
}

// authorizations returns the Authorization header of every request each host saw, in order.
//
// Returns:
//   - `[]string`: the API host's.
//   - `[]string`: the asset host's.
func (f *fakeGitHub) authorizations() (api, files []string) {

	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.apiAuth...), append([]string(nil), f.fileAuth...)
}

// serveAPI answers the three release endpoints the upgrade asks, as GitHub does.
//
// Parameters:
//   - `w`: the response.
//   - `r`: the request.
func (f *fakeGitHub) serveAPI(w http.ResponseWriter, r *http.Request) {

	f.mu.Lock()
	defer f.mu.Unlock()

	below := strings.TrimPrefix(r.URL.Path, "/repos/NobleFactor/devlore-cli")
	key := below
	if r.URL.RawQuery != "" {
		key += "?" + r.URL.RawQuery
	}
	f.requests[key]++
	f.apiAuth = append(f.apiAuth, r.Header.Get("Authorization"))

	if canned, ok := f.answers[below]; ok {
		for name, value := range canned.headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(canned.status)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": canned.message})
		return
	}

	switch {
	case below == "/releases":
		f.servePage(w, r)
	case below == "/releases/latest":
		f.serveFirst(w, func(release githubRelease) bool { return !release.Prerelease })
	case strings.HasPrefix(below, "/releases/tags/"):
		tag := strings.TrimPrefix(below, "/releases/tags/")
		f.serveFirst(w, func(release githubRelease) bool { return release.TagName == tag })
	default:
		http.NotFound(w, r)
	}
}

// servePage answers one page of the release list, drafts included, as a token with push access sees it.
//
// Parameters:
//   - `w`: the response.
//   - `r`: the request, carrying `per_page` and `page`.
func (f *fakeGitHub) servePage(w http.ResponseWriter, r *http.Request) {

	size, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if size <= 0 || page <= 0 {
		http.Error(w, "per_page and page are required", http.StatusBadRequest)
		return
	}

	start := min((page-1)*size, len(f.releases))
	end := min(start+size, len(f.releases))

	_ = json.NewEncoder(w).Encode(f.releases[start:end])
}

// serveFirst answers with the first published release that `matches`, or GitHub's 404.
//
// Parameters:
//   - `w`: the response.
//   - `matches`: which release answers.
func (f *fakeGitHub) serveFirst(w http.ResponseWriter, matches func(githubRelease) bool) {

	for _, release := range f.releases {
		if !release.Draft && matches(release) {
			_ = json.NewEncoder(w).Encode(release)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
}

// serveAsset answers an asset download by the asset's name, stalling halfway through [fakeGitHub.stall].
//
// Parameters:
//   - `w`: the response.
//   - `r`: the request.
func (f *fakeGitHub) serveAsset(w http.ResponseWriter, r *http.Request) {

	f.mu.Lock()
	f.fileAuth = append(f.fileAuth, r.Header.Get("Authorization"))
	content, ok := f.files[path.Base(r.URL.Path)]
	stall := f.stall == path.Base(r.URL.Path)
	f.mu.Unlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	if !stall {
		_, _ = w.Write(content)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	_, _ = w.Write(content[:len(content)/2])
	w.(http.Flusher).Flush()
	close(f.stalled)
	<-r.Context().Done()
}

// answerWith makes the API answer `below` with a canned status, headers and message, in place of any release.
//
// Parameters:
//   - `below`: the API path below the repository.
//   - `status`: the HTTP status.
//   - `message`: GitHub's `message`.
//   - `headers`: response headers, as name and value pairs.
func (f *fakeGitHub) answerWith(below string, status int, message string, headers ...string) {

	f.mu.Lock()
	defer f.mu.Unlock()

	canned := &cannedAnswer{status: status, message: message, headers: map[string]string{}}
	for i := 0; i+1 < len(headers); i += 2 {
		canned.headers[headers[i]] = headers[i+1]
	}
	f.answers[below] = canned
}

// channelTargetOf returns the target of a run that follows `channel`.
//
// Parameters:
//   - `channel`: develop or release.
//   - `prerelease`: whether the channel's pre-releases count.
//
// Returns:
//   - `upgradeTarget`: the target.
func channelTargetOf(channel string, prerelease bool) upgradeTarget {
	return upgradeTarget{
		mode: upgradeModeChannel, channel: channel, prerelease: prerelease || channel == channelDevelop,
	}
}

// pageQuery returns the release list's path and query for one page, as the upgrade asks for it.
//
// Parameters:
//   - `page`: the page, from 1.
//
// Returns:
//   - `string`: the path and query below the repository.
func pageQuery(page int) string {
	return fmt.Sprintf("/releases?page=%d&per_page=%d", page, releasePageSize)
}

// --- findRelease: release ---

// TestFindRelease_ReleaseAsksForTheLatest is Requirement 3: `release` is GitHub's latest release, in one request.
func TestFindRelease_ReleaseAsksForTheLatest(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bare("v0.2.0-dev.2", developRef), bare("v0.1.0", "refs/tags/v0.1.0"))

	release, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, false))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}

	if release.TagName != "v0.1.0" {
		t.Errorf("tag = %q, want v0.1.0", release.TagName)
	}
	if got := fake.apiRequests("/releases/latest"); got != 1 || fake.apiRequests("") != 1 {
		t.Errorf("%d requests for the latest, %d in all; want one", got, fake.apiRequests(""))
	}
}

// TestFindRelease_ReleaseWithNoReleaseSaysSo is Requirement 12: a 404 from latest is a channel with no release yet.
func TestFindRelease_ReleaseWithNoReleaseSaysSo(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bare("v0.2.0-dev.2", developRef))

	_, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, false))
	if err == nil {
		t.Fatal("findRelease found a release where there is none")
	}
	for _, word := range []string{"release channel has no release yet", "Not Found"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to say %q", err, word)
		}
	}
	if ExitCode(err) != ExitUnavailable {
		t.Errorf("exit %d, want %d", ExitCode(err), ExitUnavailable)
	}
}

// --- findRelease: a pin ---

// TestFindRelease_PinAsksForItsTag is Requirement 3: a pin is that exact release, whatever its channel, in one request.
func TestFindRelease_PinAsksForItsTag(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bare("v0.2.0-dev.2", developRef), bare("v0.2.0-dev.1", developRef))

	release, err := findRelease(context.Background(), fake.environment(""),
		upgradeTarget{mode: upgradeModePin, tag: "v0.2.0-dev.1"})
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}

	if release.TagName != "v0.2.0-dev.1" {
		t.Errorf("tag = %q, want the pin", release.TagName)
	}
	if got := fake.apiRequests("/releases/tags/v0.2.0-dev.1"); got != 1 || fake.apiRequests("") != 1 {
		t.Errorf("%d requests for the tag, %d in all; want one", got, fake.apiRequests(""))
	}
}

// TestFindRelease_PinNotFoundNamesTheTag is Requirement 12: a 404 from a tag names the tag.
func TestFindRelease_PinNotFoundNamesTheTag(t *testing.T) {

	fake := newFakeGitHub(t)

	pin := upgradeTarget{mode: upgradeModePin, tag: "v9.9.9"}

	_, err := findRelease(context.Background(), fake.environment(""), pin)
	if err == nil {
		t.Fatal("findRelease found a tag that does not exist")
	}
	for _, word := range []string{"v9.9.9", pinVariable, "Not Found"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to name %s", err, word)
		}
	}
}

// --- findRelease: the release list ---

// TestFindRelease_DevelopTakesTheNewestDevelopBuild is Requirement 3: one request while a develop build is newest.
func TestFindRelease_DevelopTakesTheNewestDevelopBuild(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bare("v0.2.0-dev.2", developRef), bare("v0.1.0", "refs/tags/v0.1.0"))

	release, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelDevelop, true))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}

	if release.TagName != "v0.2.0-dev.2" {
		t.Errorf("tag = %q, want v0.2.0-dev.2", release.TagName)
	}
	if got := fake.apiRequests(pageQuery(1)); got != 1 || fake.apiRequests("") != 1 {
		t.Errorf("%d requests for the first page, %d in all; want one", got, fake.apiRequests(""))
	}
}

// TestFindRelease_SkipsDrafts pins that a draft, which an owner's token sees while a release uploads, is never taken.
func TestFindRelease_SkipsDrafts(t *testing.T) {

	fake := newFakeGitHub(t)
	draft := bare("v0.2.0-dev.3", developRef)
	draft.Draft = true
	fake.publish(draft, bare("v0.2.0-dev.2", developRef))

	release, err := findRelease(context.Background(), fake.environment("owner"), channelTargetOf(channelDevelop, true))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}

	if release.TagName != "v0.2.0-dev.2" {
		t.Errorf("tag = %q; want the newest that is not a draft", release.TagName)
	}
}

// TestFindRelease_DevelopWalksPages is Requirement 3: the list is read page by page, newest first, until one is found.
func TestFindRelease_DevelopWalksPages(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bareReleases(250, 1000, "refs/heads/main")...)
	fake.publish(bare("v0.1.0-dev.1", developRef))

	release, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelDevelop, true))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}

	if release.TagName != "v0.1.0-dev.1" {
		t.Errorf("tag = %q, want v0.1.0-dev.1", release.TagName)
	}
	for page := 1; page <= 3; page++ {
		if got := fake.apiRequests(pageQuery(page)); got != 1 {
			t.Errorf("page %d asked %d times, want once", page, got)
		}
	}
	if got := fake.apiRequests(""); got != 3 {
		t.Errorf("%d requests, want 3", got)
	}
}

// TestFindRelease_PrereleaseTakesTheNewestNotFromDevelop is D1: on `release` with its pre-releases, the newest release
// whose body does not name develop, a pre-release or not.
func TestFindRelease_PrereleaseTakesTheNewestNotFromDevelop(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(
		bare("v0.2.0-dev.2", developRef), bare("v0.2.0-rc.1", "refs/heads/main"), bare("v0.1.0", "refs/tags/v0.1.0"))

	release, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, true))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}

	if release.TagName != "v0.2.0-rc.1" {
		t.Errorf("tag = %q, want v0.2.0-rc.1", release.TagName)
	}
	if got := fake.apiRequests(""); got != 1 {
		t.Errorf("%d requests, want one", got)
	}
}

// TestFindRelease_PrereleaseReadsEveryPageWhenOnlyDevelopExists is Requirement 4's arithmetic: today's 408 releases,
// every one from develop, are five pages, and `release` with its pre-releases reads all five to find none.
func TestFindRelease_PrereleaseReadsEveryPageWhenOnlyDevelopExists(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bareReleases(408, 408, developRef)...)

	_, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, true))
	if err == nil {
		t.Fatal("findRelease found a release channel build among develop's")
	}
	if !strings.Contains(err.Error(), "release channel has no release or pre-release yet") {
		t.Errorf("error = %v; want it to say what it did not find", err)
	}
	if got := fake.apiRequests(""); got != 5 {
		t.Errorf("%d requests, want 5", got)
	}
}

// TestFindRelease_TheWalkStopsAtTenPages is D9: at most 1,000 releases are read, and the error says so.
func TestFindRelease_TheWalkStopsAtTenPages(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bareReleases(1200, 1200, developRef)...)
	fake.publish(bare("v0.1.0-rc.1", "refs/heads/main"))

	_, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, true))
	if err == nil {
		t.Fatal("findRelease read past ten pages")
	}
	for _, word := range []string{"1000", "10 pages"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to say %q", err, word)
		}
	}
	if got := fake.apiRequests(""); got != releasePageLimit {
		t.Errorf("%d requests, want %d", got, releasePageLimit)
	}
}

// --- releaseChannel ---

// TestReleaseChannel_ReadsTheRefLine is D1: the channel is read from the body's `Ref:` line.
func TestReleaseChannel_ReadsTheRefLine(t *testing.T) {

	for _, c := range []struct{ body, want string }{
		{"Release v1\n\nSource: x@y\nRef: refs/heads/develop", channelDevelop},
		{"Release v1\r\n\r\nRef:   refs/heads/develop  \r\n", channelDevelop},
		{"Release v1\n\nRef: refs/heads/main", channelRelease},
		{"Release v1\n\nRef: refs/heads/release/1.2", channelRelease},
		{"Release v1\n\nRef: refs/tags/v1.2.3", channelRelease},
		{"Release v1 with no ref at all", channelRelease},
		{"Notes mention Ref: refs/heads/develop mid-line", channelRelease},
	} {
		if got := releaseChannel(c.body); got != c.want {
			t.Errorf("releaseChannel(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

// --- releaseFiles ---

// TestReleaseFiles_TakesThisPlatformsArchiveAndTheChecksums picks the two assets out of the seven.
func TestReleaseFiles_TakesThisPlatformsArchiveAndTheChecksums(t *testing.T) {

	fake := newFakeGitHub(t)
	release := fake.release(t, "v1.2.3", developRef, "lore")

	files, err := releaseFiles(release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("releaseFiles: %v", err)
	}

	if files.tag != "v1.2.3" {
		t.Errorf("tag = %q", files.tag)
	}
	if files.archive.name != archiveName("v1.2.3", runtime.GOOS, runtime.GOARCH) || files.archive.url == "" ||
		!strings.HasPrefix(files.archive.digest, "sha256:") || files.archive.path != "" {
		t.Errorf("archive = %+v", files.archive)
	}
	if files.checksums.name != checksumsName("v1.2.3") || files.checksums.url == "" {
		t.Errorf("checksums = %+v", files.checksums)
	}
}

// TestReleaseFiles_NamesBothAndTheTag is Requirement 3: a release without either file fails naming both and the tag.
func TestReleaseFiles_NamesBothAndTheTag(t *testing.T) {

	_, err := releaseFiles(bare("v1.2.3", developRef), "linux", "arm64")
	if err == nil {
		t.Fatal("releaseFiles accepted a release with no assets")
	}
	for _, word := range []string{"v1.2.3", archiveName("v1.2.3", "linux", "arm64"), checksumsName("v1.2.3")} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to name %s", err, word)
		}
	}
}

// --- the token ---

// TestFetchFile_SendsTheTokenToTheAPIOnly is Requirement 4: GH_TOKEN goes to api.github.com and nowhere else.
//
// The assets download from their public address, on another host; a token sent there would be a token given away.
func TestFetchFile_SendsTheTokenToTheAPIOnly(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v1.2.3", developRef, "lore"))
	env := fake.environment("a-token")

	release, err := findRelease(context.Background(), env, channelTargetOf(channelDevelop, true))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}
	files, err := releaseFiles(release, env.goos, env.goarch)
	if err != nil {
		t.Fatalf("releaseFiles: %v", err)
	}

	scratch, err := fsroot.OpenExisting(t.TempDir())
	if err != nil {
		t.Fatalf("OpenExisting: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	for _, file := range []releaseFile{files.checksums, files.archive} {
		if _, err := fetchFile(context.Background(), env, scratch, file); err != nil {
			t.Fatalf("fetchFile %s: %v", file.name, err)
		}
	}

	apiHeaders, assetHeaders := fake.authorizations()
	for _, header := range apiHeaders {
		if header != "Bearer a-token" {
			t.Errorf("an API request carried Authorization %q, want the token", header)
		}
	}
	if len(assetHeaders) != 2 {
		t.Fatalf("%d asset requests, want 2", len(assetHeaders))
	}
	for _, header := range assetHeaders {
		if header != "" {
			t.Errorf("an asset request carried Authorization %q; the token is for the API alone", header)
		}
	}
}

// TestFindRelease_AsksAnonymouslyWithoutAToken pins that no token means no Authorization header at all.
func TestFindRelease_AsksAnonymouslyWithoutAToken(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.publish(bare("v0.1.0", "refs/tags/v0.1.0"))

	_, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, false))
	if err != nil {
		t.Fatalf("findRelease: %v", err)
	}
	if api, _ := fake.authorizations(); len(api) != 1 || api[0] != "" {
		t.Errorf("Authorization = %q; want none", api)
	}
}

// --- fetchFile ---

// TestFetchFile_ReturnsTheSHA256OfWhatItWrote pins that the sum is of the bytes on disk, taken as they are written.
func TestFetchFile_ReturnsTheSHA256OfWhatItWrote(t *testing.T) {

	fake := newFakeGitHub(t)
	release := fake.release(t, "v1.2.3", developRef, "lore")
	files, err := releaseFiles(release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("releaseFiles: %v", err)
	}

	scratch, err := fsroot.OpenExisting(t.TempDir())
	if err != nil {
		t.Fatalf("OpenExisting: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	sum, err := fetchFile(context.Background(), fake.environment(""), scratch, files.archive)
	if err != nil {
		t.Fatalf("fetchFile: %v", err)
	}

	written, err := scratch.ReadFile(scratch.NewPath(files.archive.name))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if sum != sha256Of(written) || sum != strings.TrimPrefix(files.archive.digest, "sha256:") {
		t.Errorf("sum = %s; the file's is %s", sum, sha256Of(written))
	}
}

// TestFetchFile_ReportsAFailedDownload pins that an asset the host will not serve fails the fetch, naming it.
func TestFetchFile_ReportsAFailedDownload(t *testing.T) {

	fake := newFakeGitHub(t)

	scratch, err := fsroot.OpenExisting(t.TempDir())
	if err != nil {
		t.Fatalf("OpenExisting: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	missing := releaseFile{name: checksumsName("v1"), url: fake.assetURL("v1", checksumsName("v1"))}
	_, err = fetchFile(context.Background(), fake.environment(""), scratch, missing)
	if err == nil {
		t.Fatal("fetchFile succeeded for an asset the host does not have")
	}
	if !strings.Contains(err.Error(), missing.name) || !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v; want it to name the file and the status", err)
	}
}

// --- GitHub's errors ---

// TestFindRelease_RateLimitWithoutATokenNamesGHToken is Requirement 12: a spent anonymous limit names GH_TOKEN.
func TestFindRelease_RateLimitWithoutATokenNamesGHToken(t *testing.T) {

	for _, c := range []struct {
		label   string
		status  int
		headers []string
	}{
		{"403 remaining 0", http.StatusForbidden,
			[]string{"X-RateLimit-Remaining", "0", "X-RateLimit-Reset", "1790802357"}},
		{"429 retry-after", http.StatusTooManyRequests, []string{"Retry-After", "60"}},
		{"403 retry-after", http.StatusForbidden, []string{"Retry-After", "60"}},
	} {
		t.Run(c.label, func(t *testing.T) {

			fake := newFakeGitHub(t)
			fake.answerWith("/releases/latest", c.status, "API rate limit exceeded for 203.0.113.9.", c.headers...)

			_, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelRelease, false))
			if err == nil {
				t.Fatal("findRelease succeeded against a spent rate limit")
			}
			for _, word := range []string{tokenVariable, "rate limit", "API rate limit exceeded"} {
				if !strings.Contains(err.Error(), word) {
					t.Errorf("error = %v; want it to say %q", err, word)
				}
			}
			if ExitCode(err) != ExitTempFail {
				t.Errorf("exit %d, want %d", ExitCode(err), ExitTempFail)
			}
		})
	}
}

// TestFindRelease_RateLimitWithATokenNamesTheReset is Requirement 12: a spent limit with a token says when it resets.
func TestFindRelease_RateLimitWithATokenNamesTheReset(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.answerWith("/releases/latest", http.StatusForbidden, "API rate limit exceeded for user ID 1.",
		"X-RateLimit-Remaining", "0", "X-RateLimit-Reset", "1790802357")

	_, err := findRelease(context.Background(), fake.environment("a-token"), channelTargetOf(channelRelease, false))
	if err == nil {
		t.Fatal("findRelease succeeded against a spent rate limit")
	}

	reset := time.Unix(1790802357, 0).Local().Format(time.RFC3339)
	if !strings.Contains(err.Error(), reset) {
		t.Errorf("error = %v; want it to name the reset, %s", err, reset)
	}
	if strings.Contains(err.Error(), "set "+tokenVariable) {
		t.Errorf("error = %v; GH_TOKEN is set, so there is no token to suggest", err)
	}
}

// TestFindRelease_ARefusedTokenSaysSo is Requirement 12: a 401 says GH_TOKEN was refused.
func TestFindRelease_ARefusedTokenSaysSo(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.answerWith("/releases/latest", http.StatusUnauthorized, "Bad credentials")

	_, err := findRelease(context.Background(), fake.environment("expired"), channelTargetOf(channelRelease, false))
	if err == nil {
		t.Fatal("findRelease succeeded with a refused token")
	}
	for _, word := range []string{tokenVariable, "refused", "Bad credentials"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to say %q", err, word)
		}
	}
	if ExitCode(err) != ExitNoPerm {
		t.Errorf("exit %d, want %d", ExitCode(err), ExitNoPerm)
	}
}

// TestFindRelease_ReportsGitHubsOwnMessage is Requirement 12: any other failure reports GitHub's status and message.
func TestFindRelease_ReportsGitHubsOwnMessage(t *testing.T) {

	fake := newFakeGitHub(t)
	fake.answerWith("/releases", http.StatusForbidden, "Resource protected by organization SAML enforcement.")

	_, err := findRelease(context.Background(), fake.environment(""), channelTargetOf(channelDevelop, true))
	if err == nil {
		t.Fatal("findRelease succeeded against a 403")
	}
	for _, word := range []string{"403", "SAML enforcement"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to say %q", err, word)
		}
	}
}

// TestFindRelease_ReportsAnUnreadableAnswer pins that a 200 whose body is not a release is GitHub's protocol error.
func TestFindRelease_ReportsAnUnreadableAnswer(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>not json</html>")
	}))
	t.Cleanup(server.Close)

	env := upgradeEnvironment{apiBase: server.URL, client: server.Client(), goos: "linux", goarch: "arm64"}

	_, err := findRelease(context.Background(), env, channelTargetOf(channelRelease, false))
	if ExitCode(err) != ExitProtocol {
		t.Errorf("exit %d, %v; want a protocol error", ExitCode(err), err)
	}
}
