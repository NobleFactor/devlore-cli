#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.
#
# credentials-check.sh - prove the token this repository's workflows sign in with, and keep its incidents (#613)
#
# The credentials check's logic, which credentials-check.yml runs daily in each participating repository. It proves the
# token through the same secret the repository's workflows read, and raises one incident per condition in this
# repository, assigned to the owner, with the run's own GITHUB_TOKEN, so that it never depends on the token it checks.
# An open incident gets a comment each day its condition holds, and closes only when its own test passes: a test that
# didn't run, or that couldn't finish, clears nothing.
#
# An organization's token is a GitHub App: the check signs a JSON Web Token with the key, finds the app's installation
# on the account, mints an installation token, and requires the permissions GitHub grants it to include every one
# credentials-check.json names. A user account's is a classic token: the check calls GitHub with it, and requires its
# login to be the account, its scopes to include every one the file names, and its expiry to be beyond the margin. An
# app key isn't expired but rotated, due on TOKEN_ROTATION_DUE.
#
# Only GitHub's refusal is a credential condition: a 401, or a 403 that isn't a rate limit. GitHub not reached, a rate
# limit, a server error or any other answer proves nothing about the credential: the run ends 2, raising and clearing
# nothing, and the next one checks again (#621).
#
# The Triage workflow runs it too. When a new issue isn't added to Triage, report mode checks the token: a bad one
# raises its own alert, naming the issue missed; a healthy one raises not-triaged, "New issues aren't reaching Triage",
# naming the issue and the run to rerun. Each further miss is a comment on the same alert, and the daily check reminds
# once a day while it is open. When a later issue is added to Triage, placed mode closes it, listing the issues missed
# earlier, to check: closing an alert never adds them. An alert says only what was seen: what happened in the run, and
# what the token check found right after it, never which caused which.
#
# Environment:
#   KIND                organization or user.
#   ACCOUNT, REPOSITORY the account and this repository, owner/name.
#   GH_TOKEN            the run's GITHUB_TOKEN, with issues: write, for the incidents.
#   APP_ID, APP_PRIVATE_KEY  an organization's app, from its secrets.
#   PAT                 a user account's classic token, from its secret.
#   ROTATION_DUE        an organization's next key rotation, YYYY-MM-DD, from TOKEN_ROTATION_DUE; optional.
#   WATCH               repositories whose latest check this run also watches, comma-separated; optional.
#   MODE                check, the default; report, when a new issue wasn't added to Triage; or placed, when one was.
#   ISSUE               the new issue's number, in report and placed mode.
#   CLASSIFIED          in report mode, false when the run failed before it could tell whether the issue belongs in
#                       Triage, a schedule or a scenario; default: true.
#   RUN_URL             this run's page, named in every incident.
#   CONFIG              credentials-check.json; default: beside this script.
#   GITHUB_API_URL      GitHub's API; default: https://api.github.com.
#
# It runs on GitHub's runner from a sparse checkout of .github/credentials-check, where no engineering helper is, so
# it sources no Declare-BashScript, as a script distributed beyond this repository needn't. It checks each failure
# itself, so it sets -u and pipefail but not -e.
#
# Exit status: 0 when every condition is clear; 1 when one holds; 2 when the check couldn't finish, GitHub not reached
# or its answer proving nothing; 3 when an incident couldn't be looked up, raised, repeated or cleared. An open Triage
# alert, which the daily check can't test, is reminded of but leaves its status as it is.
set -uo pipefail

: "${KIND:?}" "${ACCOUNT:?}" "${REPOSITORY:?}" "${GH_TOKEN:?}"
config="${CONFIG:-$(dirname -- "$0")/credentials-check.json}"
api="${GITHUB_API_URL:-https://api.github.com}"
mode="${MODE:-check}"
run="${RUN_URL:-the run}"
margin="$(jq -r .margin_days "${config}")"
assignee="$(jq -r .assignee "${config}")"
guide="$(jq -r .guide "${config}")"
label=token-alert
work="$(mktemp -d)"
trap 'rm -rf -- "${work}"' EXIT

declare -A found=() cleared=()
declare -a watched=()
incomplete=0
broken=0

# raise records a condition that holds, with what was seen.
raise() {
    found["$1"]="$2"
}

# passed records conditions whose own test passed, so that their incidents may close.
passed() {
    local condition
    for condition in "$@"; do
        cleared["${condition}"]=1
    done
}

# epoch prints the UTC midnight of a date's day, YYYY-MM-DD first, in seconds; GNU or BSD date.
epoch() {
    local day="${1:0:10}"
    date -u -d "${day}" +%s 2>/dev/null || date -u -j -f "%Y-%m-%d %H:%M:%S" "${day} 00:00:00" +%s
}

# days_until prints the whole days from today to a date, negative once it has passed.
days_until() {
    local due now
    due="$(epoch "$1")" || return 1
    now="$(epoch "$(date -u +%Y-%m-%d)")" || return 1
    printf '%d\n' $(((due - now) / 86400))
}

# call makes one request of GitHub, the body to ${work}/body and the headers to ${work}/headers, and prints the HTTP
# status. A request that doesn't reach GitHub returns 2, and its caller exits 2 at once: no credential is at fault
# (#621). It returns rather than exits, since it runs in a command substitution, whose exit would end only that.
call() {
    local code
    if ! code="$(curl -sS --max-time 30 -o "${work}/body" -D "${work}/headers" -w '%{http_code}' "$@" \
        2>"${work}/curl")"; then
        printf 'GitHub not reached: %s; no credential is at fault, and the next run checks again\n' \
            "$(<"${work}/curl")" >&2
        return 2
    fi
    printf '%s\n' "${code}"
}

# header prints a response header's value, matched without regard to case.
header() {
    tr -d '\r' <"${work}/headers" | awk -v h="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" -F': ' \
        'tolower($1) == h { $1 = ""; sub(/^ /, ""); print; exit }'
}

# message prints GitHub's own message in the last answer's body, or nothing.
message() {
    jq -r '.message // empty' "${work}/body" 2>/dev/null | head -n 1
}

# refusal succeeds when the last answer is GitHub refusing the credential: a 401, or a 403 that isn't a rate limit.
refusal() {
    case "$1" in
        401) return 0 ;;
        403) [[ "$(header x-ratelimit-remaining)" != 0 ]] && ! message | grep -qi 'rate limit' ;;
        *) return 1 ;;
    esac
}

# inconclusive ends the run 2 for an answer that proves nothing about the credential: a rate limit, a server error or
# anything else GitHub's refusal isn't. Nothing is raised or cleared, and the next run checks again.
inconclusive() {
    local said
    said="$(message)"
    printf 'GitHub answered %s with HTTP %s%s, which proves nothing about the credential; nothing is raised or ' \
        "$1" "$2" "${said:+ (${said})}" >&2
    printf 'cleared, and the next run checks again\n' >&2
    exit 2
}

# b64url prints its input as unpadded base64url, as a JSON Web Token takes it.
b64url() {
    openssl base64 -A | tr '+/' '-_' | tr -d '='
}

# check_app proves an organization's app: its key signs, it is installed on the account, and the installation token
# GitHub mints holds every permission credentials-check.json names. Only all of that clears the app's conditions.
check_app() {
    if [[ -z "${APP_ID:-}" || -z "${APP_PRIVATE_KEY:-}" ]]; then
        local names="NOBLEFACTOR_AUTOMATION_APP_ID and NOBLEFACTOR_AUTOMATION_APP_PRIVATE_KEY"
        raise missing "the organization secrets ${names} aren't both set"
        return
    fi
    local key="${work}/key.pem" now head body signature jwt code installation needed short
    (umask 077 && printf '%s\n' "${APP_PRIVATE_KEY}" >"${key}")
    now="$(date -u +%s)"
    head="$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)"
    body="$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' $((now - 60)) $((now + 540)) "${APP_ID}" | b64url)"
    if ! signature="$(printf '%s.%s' "${head}" "${body}" | openssl dgst -sha256 -sign "${key}" 2>/dev/null | b64url)" ||
        [[ -z "${signature}" ]]; then
        raise refused "the private key in NOBLEFACTOR_AUTOMATION_APP_PRIVATE_KEY can't sign a token"
        return
    fi
    jwt="${head}.${body}.${signature}"
    code="$(call -H "Authorization: Bearer ${jwt}" -H "Accept: application/vnd.github+json" \
        "${api}/app/installations")" || exit 2
    if [[ "${code}" != 200 ]]; then
        refusal "${code}" || inconclusive "app ${APP_ID}'s installations" "${code}"
        raise refused "GitHub refused app ${APP_ID}'s key: HTTP ${code} ($(message))"
        return
    fi
    installation="$(jq -r --arg a "${ACCOUNT}" \
        '.[] | select((.account.login | ascii_downcase) == ($a | ascii_downcase)) | .id' "${work}/body" | head -n 1)"
    if [[ -z "${installation}" ]]; then
        raise not-installed "app ${APP_ID} isn't installed on ${ACCOUNT}"
        return
    fi
    code="$(call -X POST -H "Authorization: Bearer ${jwt}" -H "Accept: application/vnd.github+json" \
        "${api}/app/installations/${installation}/access_tokens")" || exit 2
    if [[ "${code}" == 404 ]]; then
        raise not-installed "app ${APP_ID}'s installation on ${ACCOUNT} is gone: HTTP 404"
        return
    fi
    if [[ "${code}" != 201 ]]; then
        refusal "${code}" || inconclusive "the installation token's minting" "${code}"
        raise refused "GitHub minted no installation token for app ${APP_ID}: HTTP ${code} ($(message))"
        return
    fi
    needed="$(jq -c .organization.permissions "${config}")"
    short="$(jq -r --argjson need "${needed}" '
        def rank: {"read": 1, "write": 2, "admin": 3}[.] // 0;
        .permissions as $got | $need | to_entries[] | select(($got[.key] // "" | rank) < (.value | rank))
        | "\(.key): \(.value), granted \($got[.key] // "none")"' "${work}/body" | paste -sd ';' -)"
    if [[ -n "${short}" ]]; then
        raise short "the installation token lacks ${short}"
        return
    fi
    passed missing refused not-installed short
}

# check_rotation warns before an app key's rotation date, and after it; a date beyond the margin clears both.
check_rotation() {
    [[ -n "${ROTATION_DUE:-}" ]] || return 0
    local left
    left="$(days_until "${ROTATION_DUE}")" || {
        raise rotation-due "TOKEN_ROTATION_DUE, ${ROTATION_DUE}, isn't a date"
        return
    }
    if ((left < 0)); then
        raise rotation-overdue "the app key's rotation was due on ${ROTATION_DUE:0:10}"
    elif ((left <= margin)); then
        raise rotation-due "the app key's rotation is due on ${ROTATION_DUE:0:10}, in ${left} days"
    else
        passed rotation-due rotation-overdue
    fi
}

# check_token proves a user account's classic token: it signs in as the account, with every scope, before its expiry.
# Signing in as the account with every scope clears the sign-in conditions; an expiry beyond the margin, or none,
# clears the expiry conditions.
check_token() {
    if [[ -z "${PAT:-}" ]]; then
        raise missing "the repository secret ADD_TO_PROJECT_PAT isn't set"
        return
    fi
    local code login scopes need expiry left signed_in=1
    code="$(call -H "Authorization: token ${PAT}" -H "Accept: application/vnd.github+json" "${api}/user")" || exit 2
    if [[ "${code}" != 200 ]]; then
        refusal "${code}" || inconclusive "ADD_TO_PROJECT_PAT's sign-in" "${code}"
        raise refused "GitHub refused ADD_TO_PROJECT_PAT: HTTP ${code} ($(message))"
        return
    fi
    login="$(jq -r .login "${work}/body")"
    if [[ "${login,,}" != "${ACCOUNT,,}" ]]; then
        raise wrong-account "ADD_TO_PROJECT_PAT signs in as ${login}, not ${ACCOUNT}"
        signed_in=0
    fi
    scopes=",$(header x-oauth-scopes | tr -d ' '),"
    for need in $(jq -r '.user.scopes[]' "${config}"); do
        [[ "${scopes}" == *",${need},"* ]] || {
            raise short "ADD_TO_PROJECT_PAT lacks the ${need} scope"
            signed_in=0
        }
    done
    ((signed_in == 0)) || passed missing refused wrong-account short
    expiry="$(header github-authentication-token-expiration)"
    if [[ -z "${expiry}" ]]; then
        passed expiring expired
        return
    fi
    left="$(days_until "${expiry}")" || return 0
    if ((left < 0)); then
        raise expired "ADD_TO_PROJECT_PAT expired on ${expiry}"
    elif ((left <= margin)); then
        raise expiring "ADD_TO_PROJECT_PAT expires on ${expiry}, in ${left} days"
    else
        passed expiring expired
    fi
}

# check_watched raises no-recent-check for a watched repository whose latest credentials check is over two days old,
# or that has none, and clears it for one checked within two days. A repository whose runs can't be read is neither.
check_watched() {
    local repository condition latest left
    IFS=',' read -r -a watched <<<"${WATCH:-}"
    for repository in "${watched[@]}"; do
        repository="${repository// /}"
        [[ -n "${repository}" ]] || continue
        condition="no-recent-check in ${repository}"
        if ! latest="$(gh api "repos/${repository}/actions/workflows/credentials-check.yml/runs?per_page=1" \
            --jq '.workflow_runs[0].created_at // ""' 2>"${work}/watched")"; then
            if grep -q 'HTTP 404' "${work}/watched"; then
                raise "${condition}" "${repository} has no credentials check"
            else
                printf "%s's credentials check runs couldn't be read: %s; its alert is neither raised nor cleared\n" \
                    "${repository}" "$(head -n 1 "${work}/watched")" >&2
                incomplete=1
            fi
            continue
        fi
        if [[ -z "${latest}" ]]; then
            raise "${condition}" "${repository} has no credentials check run"
        elif ! left="$(days_until "${latest}")"; then
            incomplete=1
        elif ((left < -2)); then
            raise "${condition}" "${repository}'s latest credentials check ran on ${latest:0:10}"
        else
            passed "${condition}"
        fi
    done
}

# title_of prints a condition's alert title.
title_of() {
    case "$1" in
        not-triaged) printf "New issues aren't reaching Triage" ;;
        *) printf 'Token alert: %s' "$1" ;;
    esac
}

# listed prints issue numbers as a phrase, "#1", "#1 and #2" or "#1, #2 and #3", in the sentence for their kind, $1:
# missed, an issue a run didn't add to Triage, or unknown, one a run couldn't tell the place of.
listed() {
    local kind="$1"
    shift
    local -a numbers=("$@")
    local phrase="" i
    for i in "${!numbers[@]}"; do
        if ((i == 0)); then
            phrase="#${numbers[i]}"
        elif ((i == ${#numbers[@]} - 1)); then
            phrase+=" and #${numbers[i]}"
        else
            phrase+=", #${numbers[i]}"
        fi
    done
    local one=$((${#numbers[@]} == 1))
    if [[ "${kind}" == missed ]]; then
        printf '%s %s missed earlier: check that %s in Triage, and rerun its run, linked above, if not.' "${phrase}" \
            "$( ((one)) && echo was || echo were)" "$( ((one)) && echo "it is" || echo "each is")"
    else
        printf '%s may not have been placed: check that %s where it belongs, and rerun its run, linked above, if not.' \
            "${phrase}" "$( ((one)) && echo "it is" || echo "each is")"
    fi
}

# The words that mark a line naming an issue a run didn't place, as this script writes them.
marker="\\*\\*#[0-9]+\\*\\* (wasn't added|may not have been placed)"

# history prints an alert's body and comments, one after another; it returns 1 when the alert can't be read.
history() {
    gh issue view "$1" --repo "${REPOSITORY}" --json body,comments --jq '[.body] + [.comments[].body] | .[]'
}

# missed prints the sentences naming the issues an alert recorded, all but ${ISSUE}: those a run didn't add to Triage,
# then those a run couldn't tell the place of, unless another run named them missed; or nothing. It returns 1 when the
# alert can't be read. It reads the alert alone: an issue placed by hand since is still named, as one to check.
missed() {
    local text
    local -a missing=() unknown=()
    text="$(history "$1")" || return 1
    mapfile -t missing < <(grep -oE "\\*\\*#[0-9]+\\*\\* wasn't added" <<<"${text}" | grep -oE '[0-9]+' |
        awk -v skip="${ISSUE:-}" '$0 != skip && !seen[$0]++')
    mapfile -t unknown < <(grep -oE "\\*\\*#[0-9]+\\*\\* may not have been placed" <<<"${text}" | grep -oE '[0-9]+' |
        awk -v skip="${ISSUE:-} ${missing[*]}" 'BEGIN { split(skip, s, " "); for (i in s) out[s[i]] = 1 }
            !($0 in out) && !seen[$0]++')
    ((${#missing[@]} == 0)) || printf ' %s' "$(listed missed "${missing[@]}")"
    ((${#unknown[@]} == 0)) || printf ' %s' "$(listed unknown "${unknown[@]}")"
}

# failed records an incident operation that failed: the run ends 3, and the next run tries again.
failed() {
    printf 'Token alert: %s: %s failed; the next run tries again\n' "$1" "$2" >&2
    broken=1
}

# incident_for prints the number of a condition's open incident, or nothing when there is none, and returns 1 when the
# lookup fails, never taking a failure for none. Two runs at once can each find none and each open one; the oldest is
# kept and every other closed as its duplicate, so that one is left open. Each issue a duplicate names as missed is
# carried to the one kept first, with its run, so that none is lost; a duplicate carried but not closed is carried
# again by the next run, which names each issue once all the same.
incident_for() {
    local numbers first number carried
    numbers="$(gh issue list --repo "${REPOSITORY}" --label "${label}" --state open --json number,title --limit 100 \
        --jq "[.[] | select(.title == \"$(title_of "$1")\") | .number] | sort | .[]")" || return 1
    first="$(head -n 1 <<<"${numbers}")"
    for number in $(tail -n +2 <<<"${numbers}"); do
        carried="$(history "${number}")" || return 1
        carried="$(grep -E "^${marker}" <<<"${carried}" || true)"
        if [[ -n "${carried}" ]]; then
            carried="$(printf 'Carried from #%s, which a run opened at the same time:\n\n%s\n' "${number}" \
                "${carried}")"
            gh issue comment "${first}" --repo "${REPOSITORY}" --body "${carried}" >/dev/null || return 1
        fi
        gh issue close "${number}" --repo "${REPOSITORY}" --reason completed \
            --comment "Duplicate of #${first}, opened by a run at the same time; #${first} carries this alert." \
            >/dev/null || return 1
    done
    printf '%s\n' "${first}"
}

# miss prints the line for ${ISSUE} in report mode: what happened to it in the run, and what the token check right after
# it found, the condition $1 or nothing wrong; the first such line on an alert, when $2 is first, says what to do.
miss() {
    local line
    if [[ "${CLASSIFIED:-true}" == true ]]; then
        line="**#${ISSUE}** wasn't added to the Triage project in ${run}"
    else
        line="**#${ISSUE}** may not have been placed: ${run} failed before it could tell whether #${ISSUE} belongs in"
        line+=" Triage, a schedule or a scenario"
    fi
    if [[ "$1" == not-triaged ]]; then
        line+=". A token check right after found nothing wrong with the token."
    else
        line+=". A token check right after found $1."
    fi
    [[ "$2" != first ]] || line+=" To place #${ISSUE}, rerun that run from its page once the cause is fixed."
    printf '%s' "${line}"
}

# opening prints a condition's alert body when it opens.
opening() {
    local today="$2"
    if [[ "$1" == not-triaged ]]; then
        printf '%s\n\n%s\n\n%s\n' "$(miss not-triaged first)" \
            "Each further miss is added here as a comment. This closes when a later issue reaches Triage." \
            "How the alerts work: ${guide}"
        return
    fi
    printf '%s\n\n' "**$1:** ${found[$1]}, as ${run} found on ${today}."
    [[ "${mode}" != report ]] || printf '%s\n\n' "$(miss "$1" first)"
    printf '%s\n\n%s\n' "This incident gets a comment each day it holds, and closes when a check shows it gone." \
        "How to fix it: ${guide}"
}

# keep raises, repeats or clears the incident of one condition: raised or repeated while it holds, closed only when its
# own test passed, and left as it is otherwise. In report mode each miss is a comment of its own; otherwise a held
# condition, or an open Triage alert, gets one comment a day.
keep() {
    local condition="$1" number last today still
    today="$(date -u +%Y-%m-%d)"
    number="$(incident_for "${condition}")" || {
        failed "${condition}" "looking up its incident"
        return
    }
    if [[ -n "${found[${condition}]+held}" ]]; then
        if [[ -z "${number}" ]]; then
            gh issue create --repo "${REPOSITORY}" --title "$(title_of "${condition}")" --label "${label}" \
                --assignee "${assignee}" --body "$(opening "${condition}" "${today}")" >/dev/null || {
                failed "${condition}" "opening its incident"
                return
            }
            incident_for "${condition}" >/dev/null || failed "${condition}" "closing a duplicate of its incident"
            return
        fi
        if [[ "${mode}" == report ]]; then
            still="$(miss "${condition}" again)"
            gh issue comment "${number}" --repo "${REPOSITORY}" --body "${still}" >/dev/null ||
                failed "${condition}" "commenting on #${number}"
            return
        fi
        still="Still holds on ${today}: ${found[${condition}]}. ${run}."
    elif [[ -n "${number}" && "${condition}" == not-triaged && "${mode}" == check ]]; then
        still="Still open on ${today}. Check that each issue named above is where it belongs."
        still+=" This closes when a new issue reaches it."
    elif [[ -n "${number}" && -n "${cleared[${condition}]+passed}" ]]; then
        still="$(missed "${number}")" || {
            failed "${condition}" "reading #${number}"
            return
        }
        if [[ "${condition}" == not-triaged ]]; then
            still="#${ISSUE} was added to Triage on ${today}, so new issues are reaching it again.${still}"
        else
            still="Cleared on ${today}: ${run} found it gone.${still}"
        fi
        gh issue close "${number}" --repo "${REPOSITORY}" --comment "${still}" >/dev/null ||
            failed "${condition}" "closing #${number}"
        return
    else
        return
    fi
    last="$(gh issue view "${number}" --repo "${REPOSITORY}" --json createdAt,comments \
        --jq '([.createdAt] + [.comments[].createdAt]) | max | .[0:10]')" || {
        failed "${condition}" "reading #${number}"
        return
    }
    if [[ "${last}" != "${today}" ]]; then
        gh issue comment "${number}" --repo "${REPOSITORY}" --body "${still}" >/dev/null ||
            failed "${condition}" "commenting on #${number}"
    fi
}

# The conditions that keep a token from signing in, any of which explains a new issue missing Triage; a warning ahead of
# an expiry or a rotation explains nothing.
case "${KIND}" in
    organization) causes=(missing refused not-installed short) ;;
    user) causes=(missing refused wrong-account short expired) ;;
    *)
        printf 'KIND is organization or user, not %s\n' "${KIND}" >&2
        exit 64
        ;;
esac

case "${mode}" in
    report)
        : "${ISSUE:?}"
        if [[ "${KIND}" == organization ]]; then check_app; else check_token; fi
        explained=0
        for condition in "${causes[@]}"; do
            [[ -z "${found[${condition}]+held}" ]] || explained=1
        done
        ((explained)) || raise not-triaged "#${ISSUE} wasn't added to Triage"
        conditions=("${causes[@]}" not-triaged)
        ;;
    placed)
        : "${ISSUE:?}"
        passed not-triaged
        conditions=(not-triaged)
        ;;
    check)
        if [[ "${KIND}" == organization ]]; then
            check_app
            check_rotation
            conditions=("${causes[@]}" rotation-due rotation-overdue not-triaged)
        else
            check_token
            conditions=("${causes[@]}" expiring not-triaged)
        fi
        check_watched
        for repository in "${watched[@]}"; do
            repository="${repository// /}"
            [[ -z "${repository}" ]] || conditions+=("no-recent-check in ${repository}")
        done
        ;;
    *)
        printf 'MODE is check, report or placed, not %s\n' "${mode}" >&2
        exit 64
        ;;
esac

[[ "${mode}" == placed ]] || gh label create "${label}" --repo "${REPOSITORY}" --color B60205 --force \
    --description "The credentials check found something this repository's owner must fix" >/dev/null 2>&1 || true
for condition in "${conditions[@]}"; do
    keep "${condition}"
done
for condition in "${!found[@]}"; do
    printf '%s: %s\n' "${condition}" "${found[${condition}]}"
done
((broken == 0)) || exit 3
((${#found[@]} == 0)) || exit 1
((incomplete == 0)) || exit 2
exit 0
