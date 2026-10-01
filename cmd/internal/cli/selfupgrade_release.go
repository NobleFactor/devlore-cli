// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// Finding the release a `self upgrade` installs, and fetching its two files (#947, Requirements 3, 4 and 12). The API
// is asked only to find the release: `/releases/latest` for the release channel, the release list for develop and
// for the release channel's pre-releases, and `/releases/tags/<tag>` for a pin. The files then download from their
// public addresses, which ask nothing of the API's rate limit and never see GH_TOKEN.

// githubAPIBase is the repository's root in GitHub's REST API; its releases are below it.
const githubAPIBase = "https://api.github.com/repos/NobleFactor/devlore-cli"

// githubAPIVersion is the REST API version every request asks for, so an answer's shape is the one this code reads.
const githubAPIVersion = "2022-11-28"

// tokenVariable names the environment variable whose token, when set, goes with every API request and nothing else
// (ruled 2026-09-30, docs/plans/feature/950-installers-take-base-team-and.md).
const tokenVariable = "GH_TOKEN"

// releasePageSize and releasePageLimit bound the walk of the release list: GitHub's largest page, and at most ten of
// them, 1,000 releases, so the release channel's pre-releases cannot be sought without end while develop's builds
// are all there is (#947, D9).
const (
	releasePageSize  = 100
	releasePageLimit = 10
)

// developRef is the ref a develop build's release body names on its `Ref:` line; any other ref is the release
// channel's (#947, D1).
const developRef = "refs/heads/develop"

// githubMessageLimit bounds how much of an error's body is read for GitHub's `message`.
const githubMessageLimit = 64 << 10

// region SUPPORTING TYPES

// githubRelease is the part of GitHub's release object an upgrade reads.
type githubRelease struct {
	TagName    string        `json:"tag_name"`   // the release's tag, which is the version its builds are stamped with
	Draft      bool          `json:"draft"`      // a release still uploading, visible only to a token with push access
	Prerelease bool          `json:"prerelease"` // GitHub's pre-release flag
	Body       string        `json:"body"`       // its notes, whose `Ref:` line names the ref it was built from
	Assets     []githubAsset `json:"assets"`     // the files the release carries
}

// githubAsset is the part of GitHub's asset object an upgrade reads.
type githubAsset struct {
	Name               string `json:"name"`                 // the file's name
	BrowserDownloadURL string `json:"browser_download_url"` // its public address, outside the API
	Digest             string `json:"digest"`               // GitHub's own hash of it, as "sha256:<hex>"
}

// releaseFile is one file of a release: the archive or the checksums file, on GitHub or on disk.
type releaseFile struct {
	name   string // the file's name, as the release names it
	url    string // where GitHub serves it; "" for a file on disk
	path   string // the file on disk; "" for one GitHub serves
	digest string // GitHub's "sha256:<hex>" for it; "" for a file on disk, which carries none
}

// upgradeRelease is the release a run installs: its tag, and the two files it needs of it.
type upgradeRelease struct {
	tag       string      // the release's tag
	archive   releaseFile // this platform's archive
	checksums releaseFile // the checksums file that lists it
}

// endregion

// region HELPER FUNCTIONS

// Fallible actions

// fetchFile copies one release file into `scratch`, under its own name, and returns the SHA-256 of what it wrote.
//
// A file GitHub serves is downloaded; one on disk, which `--from` names, is copied. Either way the bytes are hashed as
// they are written, so the sum verified is the sum of the copy that is unpacked. An interrupt cancels the download,
// and the error says so.
//
// Parameters:
//   - `ctx`: the run's context; canceling it stops the download.
//   - `env`: what the upgrade reaches; its client downloads.
//   - `scratch`: the run's scratch directory.
//   - `file`: the file to fetch.
//
// Returns:
//   - `string`: the SHA-256 of the copy in scratch, in hex.
//   - `error`: the file cannot be opened or downloaded, or the copy cannot be written; the run's cancellation when
//     it was interrupted.
func fetchFile(
	ctx context.Context, env upgradeEnvironment, scratch fsroot.Dir, file releaseFile,
) (sum string, err error) {

	source, err := openReleaseFile(ctx, env, file)
	if err != nil {
		return "", err
	}
	defer iox.Close(&err, source)

	copied, err := scratch.OpenFile(scratch.NewPath(file.name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("cannot write %s: %w", file.name, err)
	}
	defer iox.Close(&err, copied)

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(copied, hash), source); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		}
		return "", fmt.Errorf("fetching %s: %w", file.name, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// findRelease finds the release a target names on GitHub (#947, Requirement 3).
//
// A pin asks for its tag. The release channel without its pre-releases asks for `/releases/latest`, which GitHub
// serves only for a published release whose pre-release flag is false. Develop, and the release channel with its
// pre-releases, walk the release list for the newest on the channel.
//
// Parameters:
//   - `ctx`: the run's context.
//   - `env`: what the upgrade reaches.
//   - `target`: a pin, or a channel; never an archive.
//
// Returns:
//   - `githubRelease`: the release.
//   - `error`: GitHub's refusal, as [githubFailure] words it, or a channel with no release.
func findRelease(ctx context.Context, env upgradeEnvironment, target upgradeTarget) (githubRelease, error) {

	var release githubRelease

	switch {
	case target.mode == upgradeModePin:
		notFound := fmt.Sprintf("%s=%s names no release: GitHub has none tagged %s", pinVariable, target.tag,
			target.tag)
		err := githubGet(ctx, env, "/releases/tags/"+url.PathEscape(target.tag), notFound, &release)
		return release, err
	case target.channel == channelRelease && !target.prerelease:
		err := githubGet(ctx, env, "/releases/latest", "the release channel has no release yet", &release)
		return release, err
	default:
		return newestOnChannel(ctx, env, target.channel)
	}
}

// githubFailure words GitHub's refusal of an API request, with GitHub's own message (#947, Requirement 12).
//
// A 401 is GH_TOKEN refused. A 403 or 429 that says the rate limit is spent, by `x-ratelimit-remaining: 0` or a
// `retry-after`, is a rate limit: without a token it names GH_TOKEN, which raises the limit from 60 requests an hour,
// and with one it names when the limit resets. A 404 is what the caller says it means. Anything else is GitHub's
// status and message as they came.
//
// Parameters:
//   - `response`: GitHub's answer, not 200; its body is read for the message, and the caller closes it.
//   - `notFound`: what a 404 means for this request; "" when it means nothing more than GitHub says.
//   - `tokenSet`: whether GH_TOKEN went with the request.
//
// Returns:
//   - `error`: [ExitNoPerm] for a refused token, [ExitTempFail] for a spent rate limit, and [ExitUnavailable] for
//     everything else.
func githubFailure(response *http.Response, notFound string, tokenSet bool) error {

	said := ""
	if message := githubMessage(response.Body); message != "" {
		said = " (GitHub: " + message + ")"
	}

	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return ExitWith(ExitNoPerm, fmt.Errorf("GitHub refused %s%s: unset it, or set it to a token GitHub accepts",
			tokenVariable, said))
	case rateLimited(response) && tokenSet:
		return ExitWith(ExitTempFail, fmt.Errorf("GitHub's rate limit for %s is spent%s: it resets %s",
			tokenVariable, said, rateLimitReset(response)))
	case rateLimited(response):
		return ExitWith(ExitTempFail, fmt.Errorf("GitHub's rate limit for anonymous requests, 60 an hour, is "+
			"spent%s: set %s to a GitHub token to raise it, or try again; it resets %s", said, tokenVariable,
			rateLimitReset(response)))
	case response.StatusCode == http.StatusNotFound && notFound != "":
		return ExitWith(ExitUnavailable, fmt.Errorf("%s%s", notFound, said))
	default:
		return ExitWith(ExitUnavailable, fmt.Errorf("GitHub answered %s%s", response.Status, said))
	}
}

// githubGet asks GitHub's API for one resource below the repository, and decodes the answer into `into`.
//
// GH_TOKEN, when set, goes as the Authorization header here and nowhere else: this is the one place an API request
// is made, and downloads are made by [openReleaseFile], which never sends it. Nothing prompts.
//
// Parameters:
//   - `ctx`: the run's context.
//   - `env`: what the upgrade reaches: the API's root, the client, and the token.
//   - `below`: the resource's path below the repository, with its query.
//   - `notFound`: what a 404 means for this resource, for [githubFailure].
//   - `into`: where the answer is decoded.
//
// Returns:
//   - `error`: an [ExitUnavailable] error when GitHub cannot be reached, [githubFailure]'s for anything but a 200,
//     and an [ExitProtocol] error for a 200 that cannot be decoded.
func githubGet(ctx context.Context, env upgradeEnvironment, below, notFound string, into any) error {

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, env.apiBase+below, http.NoBody)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if env.token != "" {
		request.Header.Set("Authorization", "Bearer "+env.token)
	}

	response, err := env.client.Do(request)
	if err != nil {
		return ExitWith(ExitUnavailable, fmt.Errorf("cannot reach GitHub: %w", err))
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return githubFailure(response, notFound, env.token != "")
	}

	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return ExitWith(ExitProtocol, fmt.Errorf("GitHub's answer to %s cannot be read: %w", below, err))
	}

	return nil
}

// newestOnChannel walks GitHub's release list, newest first, for the newest published release on `channel`.
//
// Drafts are skipped: a token with push access sees a release while it uploads. A release's channel is its body's
// `Ref:` line (#947, D1). The walk stops at a page shorter than a full one, which is the list's last, or after
// [releasePageLimit] pages (D9).
//
// Parameters:
//   - `ctx`: the run's context.
//   - `env`: what the upgrade reaches.
//   - `channel`: develop, or release for the release channel with its pre-releases.
//
// Returns:
//   - `githubRelease`: the newest release on the channel.
//   - `error`: [githubGet]'s, or an [ExitUnavailable] error saying what was not found.
func newestOnChannel(ctx context.Context, env upgradeEnvironment, channel string) (githubRelease, error) {

	sought := "release"
	if channel == channelRelease {
		sought = "release or pre-release"
	}

	for page := 1; page <= releasePageLimit; page++ {
		query := url.Values{"per_page": {strconv.Itoa(releasePageSize)}, "page": {strconv.Itoa(page)}}

		var releases []githubRelease
		if err := githubGet(ctx, env, "/releases?"+query.Encode(), "", &releases); err != nil {
			return githubRelease{}, err
		}

		for _, release := range releases {
			if !release.Draft && releaseChannel(release.Body) == channel {
				return release, nil
			}
		}

		if len(releases) < releasePageSize {
			return githubRelease{}, ExitWith(ExitUnavailable, fmt.Errorf("the %s channel has no %s yet", channel,
				sought))
		}
	}

	return githubRelease{}, ExitWith(ExitUnavailable, fmt.Errorf("the %s channel has no %s among the newest %d "+
		"releases: the search stops after %d pages", channel, sought, releasePageSize*releasePageLimit,
		releasePageLimit))
}

// openReleaseFile opens a release file for reading: its download from GitHub, or the file on disk.
//
// The download goes to the file's public address with no Authorization header: GH_TOKEN is for the API alone
// (#947, Requirement 4).
//
// Parameters:
//   - `ctx`: the run's context; canceling it stops the download.
//   - `env`: what the upgrade reaches; its client downloads.
//   - `file`: the file.
//
// Returns:
//   - `io.ReadCloser`: the file's content; the caller closes it.
//   - `error`: an [ExitNoInput] error for a file on disk that cannot be opened, or an [ExitUnavailable] error for a
//     download that cannot be made or is not answered with a 200.
func openReleaseFile(ctx context.Context, env upgradeEnvironment, file releaseFile) (io.ReadCloser, error) {

	if file.path != "" {
		// Unsandboxed: `--from` names a file wherever the operator keeps it, not in a tree this program owns.
		opened, err := os.Open(file.path)
		if err != nil {
			return nil, ExitWith(ExitNoInput, err)
		}
		Note("Reading %s", file.path)
		return opened, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, file.url, http.NoBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream")

	Note("Downloading %s", file.url)
	response, err := env.client.Do(request)
	if err != nil {
		return nil, ExitWith(ExitUnavailable, fmt.Errorf("cannot download %s: %w", file.name, err))
	}

	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close() //nolint:errcheck // the status is the error; the body is not read
		return nil, ExitWith(ExitUnavailable, fmt.Errorf("cannot download %s from %s: GitHub answered %s",
			file.name, file.url, response.Status))
	}

	return response.Body, nil
}

// releaseFiles picks the two files an upgrade needs out of a release's assets: this platform's archive and the
// checksums file.
//
// Parameters:
//   - `release`: the release, as GitHub describes it.
//   - `goos`: this platform's operating system.
//   - `goarch`: this platform's architecture.
//
// Returns:
//   - `upgradeRelease`: the release's tag and the two files, with their addresses and GitHub's digests.
//   - `error`: an [ExitUnavailable] error naming both files and the tag when the release lacks either.
func releaseFiles(release githubRelease, goos, goarch string) (upgradeRelease, error) {

	files := upgradeRelease{tag: release.TagName}
	archive, checksums := archiveName(release.TagName, goos, goarch), checksumsName(release.TagName)

	for _, asset := range release.Assets {
		switch asset.Name {
		case archive:
			files.archive = releaseFile{name: asset.Name, url: asset.BrowserDownloadURL, digest: asset.Digest}
		case checksums:
			files.checksums = releaseFile{name: asset.Name, url: asset.BrowserDownloadURL, digest: asset.Digest}
		}
	}

	if files.archive.url == "" || files.checksums.url == "" {
		return upgradeRelease{}, ExitWith(ExitUnavailable, fmt.Errorf("release %s does not carry both %s and %s, "+
			"which an upgrade on %s/%s needs", release.TagName, archive, checksums, goos, goarch))
	}

	return files, nil
}

// Actions

// githubMessage reads GitHub's `message` from an error's body.
//
// Parameters:
//   - `body`: the response's body.
//
// Returns:
//   - `string`: the message; "" when the body carries none.
func githubMessage(body io.Reader) string {

	var answer struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(body, githubMessageLimit)).Decode(&answer); err != nil {
		return ""
	}

	return strings.TrimSpace(answer.Message)
}

// rateLimited reports whether GitHub refused a request because a rate limit is spent.
//
// GitHub says so with a 403 or a 429, carrying `x-ratelimit-remaining: 0` for the primary limit or `retry-after`
// for a secondary one.
//
// Parameters:
//   - `response`: GitHub's answer.
//
// Returns:
//   - `bool`: true for a spent rate limit.
func rateLimited(response *http.Response) bool {

	if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusTooManyRequests {
		return false
	}

	return response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != ""
}

// rateLimitReset says when a spent rate limit lets requests through again.
//
// Parameters:
//   - `response`: GitHub's answer.
//
// Returns:
//   - `string`: `at <time>` from `x-ratelimit-reset`, local, RFC 3339; `in <n> seconds` from `retry-after`; or `within
//     the hour` when GitHub says neither.
func rateLimitReset(response *http.Response) string {

	if reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		return "at " + time.Unix(reset, 0).Local().Format(time.RFC3339)
	}

	if after := response.Header.Get("Retry-After"); after != "" {
		return "in " + after + " seconds"
	}

	return "within the hour"
}

// releaseChannel returns the channel a release belongs to, from its body's `Ref:` line (#947, D1).
//
// release.yaml writes the ref a release was built from into its body. `refs/heads/develop` is develop's; any other
// ref, and a body with no `Ref:` line, is the release channel's.
//
// Parameters:
//   - `body`: the release's notes.
//
// Returns:
//   - `string`: develop or release.
func releaseChannel(body string) string {

	for line := range strings.SplitSeq(body, "\n") {
		if ref, found := strings.CutPrefix(strings.TrimSpace(line), "Ref:"); found {
			if strings.TrimSpace(ref) == developRef {
				return channelDevelop
			}
			return channelRelease
		}
	}

	return channelRelease
}

// endregion
