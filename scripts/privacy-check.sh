#!/usr/bin/env bash
# Stops personal data reaching a public repository. Runs before every commit
# (.githooks, set up by `make hooks`) and in CI on the whole tree.
#
#   scripts/privacy-check.sh            every tracked file
#   scripts/privacy-check.sh --staged   what is about to be committed
#   scripts/privacy-check.sh --message FILE   a commit message
#
# Your own private words (your number, name, group names) go in
# .git/info/privacy-words, one per line: that file can never be committed.
# Example numbers: use the 1555 range, or add them to scripts/privacy-allow.txt.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

fails=0
fail() { printf 'privacy: %s\n' "$*" >&2; fails=$((fails + 1)); }

private_words="$(git rev-parse --git-path info/privacy-words)"

check_words() { # label, text on stdin
  [[ -s "$private_words" ]] || return 0
  if grep -qiFf <(grep -v '^[[:space:]]*$' "$private_words") -; then
    fail "$1 contains one of your private words (.git/info/privacy-words)"
  fi
}

if [[ "${1:-}" == "--message" ]]; then
  check_words "the commit message" < "$2"
  exit $(( fails > 0 ))
fi

if [[ "${1:-}" == "--staged" ]]; then
  files="$(git diff --cached --name-only --diff-filter=ACMR)"
  read_file() { git show ":$1"; }
else
  files="$(git ls-files)"
  read_file() { cat "$1"; }
fi

# Files that only ever hold someone's data, and models people download for
# themselves (see the Translator section of the README).
blocked='(^|/)(whatsapp\.db.*|.*\.db|.*\.db-.*|.*\.sqlite.*|secrets\.json|instance\.json|chats\.json|personas\.json|approvals\.json|runtime\.json|calendars\.json|recaps\.json|whatsapp_contacts\.json|target_cache\.json|.*\.jsonl|.*\.log|\.env(\..*)?|.*\.pem|.*\.key|.*\.p12|.*\.argosmodel|.*\.whl|model\.bin|sentencepiece\.model)$|^data/|(^|/)(translator|venv|\.venv)/|(^|/)(history|memories|decisions|summaries|briefs|ratings)/.*\.json$'

keys='sk-ant-[A-Za-z0-9_-]{16,}|sk-[A-Za-z0-9_-]{20,}|ghp_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{20,}|AIza[0-9A-Za-z_-]{30,}|AKIA[0-9A-Z]{16}|xox[baprs]-[0-9A-Za-z-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY'
emails='[A-Za-z0-9._%+-]+@(gmail|googlemail|outlook|hotmail|live|yahoo|icloud|me|proton|protonmail|walla|campus\.technion\.ac)\.[a-z.]+'
homes='(/Users/|/home/|C:\\Users\\)[A-Za-z][A-Za-z0-9._-]+[/\\]'
numbers='[0-9]{10,20}(-[0-9]{10})?@(s\.whatsapp\.net|g\.us|lid)|\+?972[- ]?5[0-9][- ]?[0-9]{3}[- ]?[0-9]{4}|05[0-9]-[0-9]{3}-?[0-9]{4}'

allow="$(tr -d ' -+' < scripts/privacy-allow.txt)"
real_numbers() { # stdin: found numbers; stdout: the ones not allowed
  while IFS= read -r n; do
    d="$(printf '%s' "${n%@*}" | tr -cd '0-9')"
    [[ "$d" == 1555* ]] && continue
    grep -qxF "$d" <<<"$allow" || printf '%s\n' "$n"
  done
}

while IFS= read -r f; do
  [[ -n "$f" ]] || continue
  if grep -qE "$blocked" <<<"$f" && [[ "$f" != ".env.example" ]]; then
    fail "$f: this kind of file holds private data and must not be committed"
    continue
  fi
  case "$f" in scripts/privacy-check.sh|scripts/privacy-allow.txt) continue ;; esac
  text="$(read_file "$f" | LC_ALL=C tr -d '\000')" || continue
  if LC_ALL=C grep -qE "$keys" <<<"$text"; then fail "$f: looks like an API key or private key"; fi
  mail="$(LC_ALL=C grep -oE "$emails" <<<"$text" | grep -v noreply | head -1 || true)"
  [[ -z "$mail" ]] || fail "$f: personal email address ($mail)"
  if LC_ALL=C grep -qE "$homes" <<<"$text"; then fail "$f: a path into someone's home folder"; fi
  found="$(LC_ALL=C grep -oE "$numbers" <<<"$text" | sort -u | real_numbers || true)"
  [[ -z "$found" ]] || fail "$f: a real-looking phone number or WhatsApp id ($(head -1 <<<"$found")). Use the 1555 range for examples"
  check_words "$f" <<<"$text"
done <<<"$files"

if [[ "${1:-}" == "--staged" ]]; then
  case "$(git config user.email)" in
    *noreply*) ;;
    *) printf 'privacy: note: commits show your email (%s). See CONTRIBUTING.md to keep it private.\n' "$(git config user.email)" >&2 ;;
  esac
fi

if ((fails > 0)); then
  printf 'privacy: %d problem(s). Nothing personal may go into this public repository.\n' "$fails" >&2
  exit 1
fi
printf 'privacy: clean\n'
