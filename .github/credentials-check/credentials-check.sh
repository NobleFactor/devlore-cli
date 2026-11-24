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
# An organization's app must reach this repository and every other one its workflows write to with the app, which the
# repository variable TOKEN_CHECK_TARGETS names: GitHub mints a token for each repository alone, and refuses one the
# app's installation doesn't cover. A refusal raises unreachable, naming each repository; only every one reached
# clears it. Every run in a repository, the daily check, Triage's report and a workflow's own, reads the same list.
#
# Where the repository variable AZURE_STATIC_SITE names a static site, the Azure credentials its deployment signs in
# with are checked too: AZURE_CREDENTIALS, the service principal azure/login reads, must sign in, and its secret's
# expiry, which the repository variable AZURE_CREDENTIALS_EXPIRES records, must be beyond the margin;
# AZURE_STATIC_WEB_APPS_API_TOKEN must be the site's current deployment token, which the service principal reads from
# Azure. Azure not reached, or answering with something that proves nothing, leaves the sign-in's and the deployment
# token's alerts as they are; the expiry's follow the recorded date, which needs nothing of Azure. A run that finds
# something wrong ends 1, even when part of it couldn't be checked.
#
# Any workflow that signs in can report its own failure: report mode without ISSUE checks what CHECKS names at once,
# and a bad credential raises its alert, naming the workflow and its run; a credential that checks fine raises nothing,
# the failure being the workflow's own.
#
# Environment:
#   KIND                organization or user.
#   ACCOUNT, REPOSITORY the account and this repository, owner/name.
#   GH_TOKEN            the run's GITHUB_TOKEN, with issues: write, for the incidents.
#   APP_ID, APP_PRIVATE_KEY  an organization's app, from its secrets.
#   PAT                 a user account's classic token, from its secret.
#   ROTATION_DUE        an organization's next key rotation, YYYY-MM-DD, from TOKEN_ROTATION_DUE; optional.
#   WATCH               repositories whose latest check this run also watches, comma-separated; optional.
#   TARGETS             the other repositories, in the account, this repository's workflows write to with the app,
#                       comma-separated, from TOKEN_CHECK_TARGETS; optional. This repository is always reached too.
#   MODE                check, the default; report, when a new issue wasn't added to Triage or a workflow failed; or
#                       placed, when a new issue was added to Triage.
#   ISSUE               the new issue's number: in report mode, a Triage miss; in placed mode, the issue placed.
#   WORKFLOW            in report mode without ISSUE, the workflow that failed.
#   CHECKS              in report mode without ISSUE, what the workflow signs in with, comma-separated: app, the
#                       organization's GitHub App; token, the user account's classic token; azure, the Azure
#                       credentials.
#   AZURE_STATIC_SITE   the static site whose deployment signs in to Azure; unset, no Azure check.
#   AZURE_CREDENTIALS   the service principal's JSON, as azure/login reads it.
#   AZURE_CREDENTIALS_EXPIRES  the date its secret expires, YYYY-MM-DD.
#   AZURE_STATIC_WEB_APPS_API_TOKEN  the site's deployment token.
#   AZURE_LOGIN_URL, AZURE_API_URL  Azure's sign-in and management APIs; defaults: https://login.microsoftonline.com and
#                       https://management.azure.com.
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
azure_login="${AZURE_LOGIN_URL:-https://login.microsoftonline.com}"
azure_api="${AZURE_API_URL:-https://management.azure.com}"
mode="${MODE:-check}"
run="${RUN_URL:-the run}"
margin="$(jq -r .margin_days "${config}")"
assignee="$(jq -r .assignee "${config}")"
guide="$(jq -r .guide "${config}")"
label=token-alert
work="$(mktemp -d)"
trap 'rm -rf -- "${work}"' EXIT

declare -A found=() cleared=()
# The organization app's installation token, once minted: setting an issue field takes push access, which the run's own
# token lacks.
app_token=""
declare -a watched=()
incomplete=0
broken=0

# raise records a condition that holds, with what was seen.
raise() {
    local condition="$1"
    shift
    found["${condition}"]="$*"
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

# call makes one request of GitHub or Azure, the body to ${work}/body and the headers to ${work}/headers, and prints the
# HTTP status. A request that doesn't reach its service returns 2, and no credential is at fault (#621): a GitHub
# request's caller exits 2 at once, and an Azure request's leaves the Azure sign-in's and deployment token's alerts as
# they are. It returns rather than exits, since it runs in a command substitution, whose exit would end only that.
call() {
    local code
    if ! code="$(curl -sS --max-time 30 -o "${work}/body" -D "${work}/headers" -w '%{http_code}' "$@" \
        2>"${work}/curl")"; then
        local url="${!#}"
        url="${url#*://}"
        printf '%s not reached: %s; no credential is at fault, and the next run checks again\n' "${url%%/*}" \
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

# app_jwt prints a JSON Web Token for the organization's app, signed with its private key, or returns 1 when the key
# can't sign one.
app_jwt() {
    local key="${work}/key.pem" now head body signature
    (umask 077 && printf '%s\n' "${APP_PRIVATE_KEY}" >"${key}")
    now="$(date -u +%s)"
    head="$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)"
    body="$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' $((now - 60)) $((now + 540)) "${APP_ID}" | b64url)"
    signature="$(printf '%s.%s' "${head}" "${body}" | openssl dgst -sha256 -sign "${key}" 2>/dev/null | b64url)" ||
        return 1
    [[ -n "${signature}" ]] || return 1
    printf '%s.%s.%s\n' "${head}" "${body}" "${signature}"
}

# check_app proves an organization's app: its key signs, it is installed on the account, and the installation token
# GitHub mints holds every permission credentials-check.json names. Only all of that clears the app's conditions.
check_app() {
    if [[ -z "${APP_ID:-}" || -z "${APP_PRIVATE_KEY:-}" ]]; then
        local names="NOBLEFACTOR_AUTOMATION_APP_ID and NOBLEFACTOR_AUTOMATION_APP_PRIVATE_KEY"
        raise missing "the organization secrets ${names} aren't both set"
        return
    fi
    local jwt code installation needed short
    if ! jwt="$(app_jwt)"; then
        raise refused "the private key in NOBLEFACTOR_AUTOMATION_APP_PRIVATE_KEY can't sign a token"
        return
    fi
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
    app_token="$(jq -r '.token // ""' "${work}/body")"
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
    check_reach "${jwt}" "${installation}"
}

# check_reach proves the app reaches this repository and every one TARGETS names: GitHub mints a token for each
# repository alone, and refuses one its installation doesn't cover, with HTTP 422. Every repository reached clears
# unreachable; an answer that proves nothing ends the run 2.
check_reach() {
    local jwt="$1" installation="$2" name code list
    local -a names=("${REPOSITORY#*/}") targets=() missed=()
    IFS=',' read -r -a targets <<<"${TARGETS:-}"
    for name in "${targets[@]}"; do
        name="${name// /}"
        [[ -n "${name}" ]] || continue
        if [[ "${name}" == */* ]]; then
            if [[ "${name%%/*}" != "${ACCOUNT}" ]]; then
                printf 'TOKEN_CHECK_TARGETS names %s, outside %s, which its app can never reach\n' "${name}" \
                    "${ACCOUNT}" >&2
                exit 64
            fi
            name="${name#*/}"
        fi
        names+=("${name}")
    done
    mapfile -t names < <(printf '%s\n' "${names[@]}" | sort -u)
    for name in "${names[@]}"; do
        jq -n --arg r "${name}" '{repositories: [$r]}' >"${work}/request"
        code="$(call -X POST -H "Authorization: Bearer ${jwt}" -H "Accept: application/vnd.github+json" \
            -H "Content-Type: application/json" --data "@${work}/request" \
            "${api}/app/installations/${installation}/access_tokens")" || exit 2
        case "${code}" in
            201) ;;
            422) missed+=("${ACCOUNT}/${name}") ;;
            *)
                refusal "${code}" || inconclusive "a token for ${ACCOUNT}/${name} alone" "${code}"
                missed+=("${ACCOUNT}/${name}")
                ;;
        esac
    done
    if ((${#missed[@]} == 0)); then
        passed unreachable
        return
    fi
    printf -v list '%s, ' "${missed[@]}"
    raise unreachable "app ${APP_ID} can't reach ${list%, }: GitHub refused it a token for each"
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

# azure_skipped says why the Azure check stopped short, leaving the sign-in's and the deployment token's alerts as they
# are, and marks the run incomplete: the service answering with something that proves nothing about the credentials.
azure_skipped() {
    printf 'Azure answered %s with HTTP %s, which proves nothing about the credentials;' "$1" "$2" >&2
    printf " the sign-in's and the deployment token's alerts stay as they are\n" >&2
    incomplete=1
}

# check_azure_expiry warns before the Azure service principal's secret expires, and after, from the date the repository
# variable AZURE_CREDENTIALS_EXPIRES records; a date beyond the margin clears both.
check_azure_expiry() {
    local left
    if [[ -z "${AZURE_CREDENTIALS_EXPIRES:-}" ]]; then
        raise azure-expiry-unset "the repository variable AZURE_CREDENTIALS_EXPIRES, the date" \
            "AZURE_CREDENTIALS's secret expires, isn't set"
        return
    fi
    left="$(days_until "${AZURE_CREDENTIALS_EXPIRES}")" || {
        raise azure-expiry-unset "AZURE_CREDENTIALS_EXPIRES, ${AZURE_CREDENTIALS_EXPIRES}, isn't a date"
        return
    }
    if ((left < 0)); then
        raise azure-expired "AZURE_CREDENTIALS's secret expired on ${AZURE_CREDENTIALS_EXPIRES:0:10}"
    elif ((left <= margin)); then
        raise azure-expiring "AZURE_CREDENTIALS's secret expires on ${AZURE_CREDENTIALS_EXPIRES:0:10}, in ${left} days"
    else
        passed azure-expiring azure-expired azure-expiry-unset
    fi
}

# check_azure proves the Azure credentials the static site AZURE_STATIC_SITE deploys with: the service principal signs
# in, and the deployment token stored is the site's current one. Signing in clears the sign-in conditions; the token
# matching clears the deployment token's.
check_azure() {
    local client secret tenant subscription code access site said
    check_azure_expiry
    if [[ -z "${AZURE_CREDENTIALS:-}" ]]; then
        raise azure-missing "the repository secret AZURE_CREDENTIALS isn't set"
        return
    fi
    client="$(jq -r '.clientId // empty' <<<"${AZURE_CREDENTIALS}" 2>/dev/null)"
    secret="$(jq -r '.clientSecret // empty' <<<"${AZURE_CREDENTIALS}" 2>/dev/null)"
    tenant="$(jq -r '.tenantId // empty' <<<"${AZURE_CREDENTIALS}" 2>/dev/null)"
    subscription="$(jq -r '.subscriptionId // empty' <<<"${AZURE_CREDENTIALS}" 2>/dev/null)"
    if [[ -z "${client}" || -z "${secret}" || -z "${tenant}" || -z "${subscription}" ]]; then
        raise azure-refused "AZURE_CREDENTIALS isn't the JSON azure/login reads: it needs clientId, clientSecret," \
            "tenantId and subscriptionId"
        return
    fi
    (umask 077 && jq -rn --arg c "${client}" --arg s "${secret}" --arg scope "${azure_api}/.default" \
        '"client_id=\($c|@uri)&client_secret=\($s|@uri)&grant_type=client_credentials&scope=\($scope|@uri)"' \
        >"${work}/azure-form")
    code="$(call -X POST -H "Content-Type: application/x-www-form-urlencoded" --data "@${work}/azure-form" \
        "${azure_login}/${tenant}/oauth2/v2.0/token")" || {
        incomplete=1
        return
    }
    if [[ "${code}" == 400 || "${code}" == 401 ]]; then
        said="$(jq -r '.error_description // .error // empty' "${work}/body" 2>/dev/null | head -n 1 | cut -c1-200)"
        raise azure-refused "Azure refused AZURE_CREDENTIALS: HTTP ${code}${said:+ (${said})}"
        return
    fi
    access="$(jq -r '.access_token // empty' "${work}/body" 2>/dev/null)"
    if [[ "${code}" != 200 || -z "${access}" ]]; then
        azure_skipped "the service principal's sign-in" "${code}"
        return
    fi
    passed azure-missing azure-refused
    if [[ -z "${AZURE_STATIC_WEB_APPS_API_TOKEN:-}" ]]; then
        raise deploy-token-missing "the repository secret AZURE_STATIC_WEB_APPS_API_TOKEN isn't set"
        return
    fi
    code="$(call -H "Authorization: Bearer ${access}" \
        "${azure_api}/subscriptions/${subscription}/providers/Microsoft.Web/staticSites?api-version=2022-09-01")" || {
        incomplete=1
        return
    }
    case "${code}" in
        200) ;;
        401 | 403)
            raise azure-short "AZURE_CREDENTIALS can't read the subscription's static sites: HTTP ${code}"
            return
            ;;
        *)
            azure_skipped "the list of static sites" "${code}"
            return
            ;;
    esac
    site="$(jq -r --arg n "${AZURE_STATIC_SITE}" '.value[]? | select(.name == $n) | .id' "${work}/body" | head -n 1)"
    if [[ -z "${site}" ]]; then
        raise azure-short "AZURE_CREDENTIALS sees no static site named ${AZURE_STATIC_SITE}"
        return
    fi
    code="$(call -X POST -H "Authorization: Bearer ${access}" -H "Content-Length: 0" \
        "${azure_api}${site}/listSecrets?api-version=2022-09-01")" || {
        incomplete=1
        return
    }
    case "${code}" in
        200) ;;
        401 | 403)
            raise azure-short "AZURE_CREDENTIALS can't read ${AZURE_STATIC_SITE}'s deployment token: HTTP ${code}"
            return
            ;;
        *)
            azure_skipped "${AZURE_STATIC_SITE}'s deployment token" "${code}"
            return
            ;;
    esac
    if [[ "$(jq -r '.properties.apiKey // empty' "${work}/body")" == "${AZURE_STATIC_WEB_APPS_API_TOKEN}" ]]; then
        passed azure-short deploy-token-missing deploy-token-stale
    else
        raise deploy-token-stale "AZURE_STATIC_WEB_APPS_API_TOKEN isn't ${AZURE_STATIC_SITE}'s current deployment" \
            "token: it was reset in Azure after it was stored"
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

# The words that mark a line a report writes, an issue's or a workflow's, which a duplicate carries.
reported="\\*\\*[^*]+\\*\\* (wasn't added|may not have been placed|failed in)"

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

# mint_token mints the organization app's installation token into app_token, for writing an issue field, and returns 1
# when it can't: the secrets unset, the key refused, the app not installed, or GitHub not answering. It raises nothing:
# proving the app is the credentials check's work.
mint_token() {
    [[ -z "${app_token}" ]] || return 0
    [[ -n "${APP_ID:-}" && -n "${APP_PRIVATE_KEY:-}" ]] || return 1
    local jwt code installation
    jwt="$(app_jwt)" || return 1
    code="$(call -H "Authorization: Bearer ${jwt}" -H "Accept: application/vnd.github+json" \
        "${api}/app/installations")" || return 1
    [[ "${code}" == 200 ]] || return 1
    installation="$(jq -r --arg a "${ACCOUNT}" \
        '.[] | select((.account.login | ascii_downcase) == ($a | ascii_downcase)) | .id' "${work}/body" | head -n 1)"
    [[ -n "${installation}" ]] || return 1
    code="$(call -X POST -H "Authorization: Bearer ${jwt}" -H "Accept: application/vnd.github+json" \
        "${api}/app/installations/${installation}/access_tokens")" || return 1
    [[ "${code}" == 201 ]] || return 1
    app_token="$(jq -r '.token // ""' "${work}/body")"
    [[ -n "${app_token}" ]]
}

# resolve sets a duplicate alert's Resolution to Duplicate, as the issue standard has every duplicate closed: in a user
# account, the resolution:duplicate label, with the run's own token; in an organization, its Resolution issue field,
# with the app's installation token. It returns 1 when it can't.
resolve() {
    local number="$1" field code
    if [[ "${KIND}" == user ]]; then
        gh issue edit "${number}" --repo "${REPOSITORY}" --add-label resolution:duplicate >/dev/null
        return
    fi
    mint_token || return 1
    code="$(call -H "Authorization: Bearer ${app_token}" -H "Accept: application/vnd.github+json" \
        "${api}/orgs/${ACCOUNT}/issue-fields")" || return 1
    [[ "${code}" == 200 ]] || return 1
    field="$(jq -r '.[] | select(.name == "Resolution") | .id' "${work}/body" | head -n 1)"
    [[ -n "${field}" ]] || return 1
    jq -cn --argjson f "${field}" '{issue_field_values: [{field_id: $f, value: "Duplicate"}]}' >"${work}/request"
    code="$(call -X POST -H "Authorization: Bearer ${app_token}" -H "Accept: application/vnd.github+json" \
        -H "Content-Type: application/json" --data "@${work}/request" \
        "${api}/repos/${REPOSITORY}/issues/${number}/issue-field-values")" || return 1
    [[ "${code}" == 2* ]]
}

# incident_for prints the number of a condition's open incident, or nothing when there is none, and returns 1 when the
# lookup fails, never taking a failure for none. Two runs at once can each find none and each open one; the oldest is
# kept and every other closed as its duplicate, so that one is left open. Each line a duplicate's reports wrote, an
# issue missed or a workflow failed, is carried to the one kept first, unless it has the line already: a retry carries
# what was added since, and repeats nothing a carry whose answer was lost delivered. A duplicate's Resolution is set to
# Duplicate before it is closed; one whose Resolution can't be set stays open, marked in ${work}/unresolved, and the
# next run tries again.
incident_for() {
    local numbers first number carried kept fresh line
    numbers="$(gh issue list --repo "${REPOSITORY}" --label "${label}" --state open --json number,title --limit 100 \
        --jq "[.[] | select(.title == \"$(title_of "$1")\") | .number] | sort | .[]")" || return 1
    first="$(head -n 1 <<<"${numbers}")"
    for number in $(tail -n +2 <<<"${numbers}"); do
        carried="$(history "${number}")" || return 1
        kept="$(history "${first}")" || return 1
        fresh=""
        while IFS= read -r line; do
            [[ -z "${line}" ]] || grep -qxF -- "${line}" <<<"${kept}" || fresh+="${line}"$'\n'
        done < <(grep -E "^${reported}" <<<"${carried}" || true)
        if [[ -n "${fresh}" ]]; then
            carried="$(printf 'Carried from #%s, which a run opened at the same time:\n\n%s' "${number}" "${fresh}")"
            gh issue comment "${first}" --repo "${REPOSITORY}" --body "${carried}" >/dev/null || return 1
        fi
        if ! resolve "${number}"; then
            printf "#%s, a duplicate of #%s, stays open: its Resolution couldn't be set to Duplicate\n" "${number}" \
                "${first}" >&2
            touch "${work}/unresolved"
            continue
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
    if [[ -z "${ISSUE:-}" ]]; then
        line="**${WORKFLOW}** failed in ${run}. A check right after found $1."
        [[ "$2" != first ]] || line+=" Rerun it from its page once the cause is fixed."
        printf '%s' "${line}"
        return
    fi
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
    organization) causes=(missing refused not-installed short unreachable) ;;
    user) causes=(missing refused wrong-account short expired) ;;
    *)
        printf 'KIND is organization or user, not %s\n' "${KIND}" >&2
        exit 64
        ;;
esac
azure_causes=(azure-missing azure-refused azure-short azure-expired deploy-token-missing deploy-token-stale)
azure_conditions=("${azure_causes[@]}" azure-expiring azure-expiry-unset)

case "${mode}" in
    report)
        if [[ -z "${ISSUE:-}" ]]; then
            : "${WORKFLOW:?}" "${CHECKS:?}"
            conditions=()
            for check in ${CHECKS//,/ }; do
                case "${check}" in
                    app)
                        check_app
                        conditions+=(missing refused not-installed short unreachable)
                        ;;
                    token)
                        check_token
                        conditions+=(missing refused wrong-account short expired)
                        ;;
                    azure)
                        check_azure
                        conditions+=("${azure_causes[@]}")
                        ;;
                    *)
                        printf 'CHECKS names app, token or azure, not %s\n' "${check}" >&2
                        exit 64
                        ;;
                esac
            done
        else
            if [[ "${KIND}" == organization ]]; then check_app; else check_token; fi
            explained=0
            for condition in "${causes[@]}"; do
                [[ -z "${found[${condition}]+held}" ]] || explained=1
            done
            ((explained)) || raise not-triaged "#${ISSUE} wasn't added to Triage"
            conditions=("${causes[@]}" not-triaged)
        fi
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
        if [[ -n "${AZURE_STATIC_SITE:-}" ]]; then
            check_azure
            conditions+=("${azure_conditions[@]}")
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
[[ ! -e "${work}/unresolved" ]] || failed "a duplicate alert" "setting its Resolution"
((broken == 0)) || exit 3
((${#found[@]} == 0)) || exit 1
((incomplete == 0)) || exit 2
exit 0
