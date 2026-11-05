# Safe software habits

The same guide is meant for people and for coding agents working on this or any other project.

What goes wrong when building software, and what to do instead. Each rule comes from a real failure or a
common one in the same area. Read the section for the work at hand, and run the checklist before anything
leaves the machine.

## Before anything leaves the machine

- [ ] Nothing personal in the diff, the commit message, the PR text or the commit's author email.
- [ ] New files are ones you meant to add. Read the file list, not only the summary count.
- [ ] Permissions and defaults are the safest reasonable ones.
- [ ] The behaviour was exercised for real, in the default configuration, not only in the changed path.
- [ ] Every "done" you are about to report was verified, and every limit is stated.

## Secrets and personal data

**Assume everything committed is public forever.** Removing it later is expensive and often incomplete.

- Keep user data outside the project folder by default: sessions, databases, keys, chat history, config.
- Use a whitelist `.gitignore` (ignore everything, re-include source folders). A blacklist fails open on the
  first new file type. Then block data file types again after the re-includes, so they stay blocked inside
  source folders.
- Personal data hides beyond file contents. Check all of these:
  - commit author and committer email, and git config defaults
  - app and bundle identifiers, package names, default usernames
  - placeholders, examples and test fixtures that use a real name, number or address
  - absolute home-folder paths in docs, logs and generated files
  - screenshots, recordings and pasted logs
  - PR descriptions, issue text and commit messages
- Use obviously fake fixtures: reserved phone ranges, `example.com`, neutral names. Never copy real data
  into tests, not even for a quick reproduction.
- Keep private words (your own names, numbers, group names) in a local list that cannot be committed, such as
  `.git/info/`, and scan against it.
- Automate the check at two levels: a pre-commit hook, which can be skipped, and CI, which cannot. Hooks are
  not active after cloning unless setup says so. A check in CI runs after a push, so the data may already be
  public on a branch.
- Run privacy checks on every change. A CI workflow that skips docs-only changes also skips the place where
  personal details most often slip in, so give the check its own workflow with no path filters.
- Use a noreply commit email from the first commit, before the repository is public.
- Compiled files keep what their source held. A PDF can carry names or IDs inside compressed streams, so a text
  search of the repository won't find them. Decompress and check before publishing.
- Mask secrets in every API response and UI. Keep credential files readable only by their owner.
- Error messages and logs must not carry tokens, full URLs with secrets, message text or personal details.
  Show the hostname at most.
- Secrets belong in the environment or a secret store, not in code, command-line arguments or URLs.

**Copies are leaks waiting to happen.** Backups, scratch folders, temp directories and test sandboxes that
hold a real session or database are live credentials. Before copying one, ask whether a copy is needed.
If it is, record where it is and delete it as soon as it has served its purpose. When auditing, search
temp and scratch locations too, including other tools' folders, and test whether a file is a real credential
rather than guessing from its name.

## Git history and public repositories

- Audit all refs and all history, not only the current tree. A file deleted last year is still public.
- Search commit metadata separately: `git log --all --format='%ae%n%ce' | sort | uniq -c`.
- Rewriting history is not the same as removing data. On GitHub, merged PRs keep their own references to old
  commits, so old commit URLs and `.patch` links keep working after a force-push. Forks and clones also
  keep the data. After any rewrite, fetch an old commit by its ID to see what is really gone.
- If a real secret was ever pushed, rotate it. Treat it as compromised; rewriting does not un-leak it.
- History rewrites: work in a fresh mirror clone, make a verified backup first, and compare file trees and
  commit counts before and after. Push with `--force-with-lease` per ref, never plain `--force`. Tell anyone
  else working on the repository, before and after.
- Old branches keep old history. Merging one brings removed data straight back. Cherry-pick instead.

## Permissions and data minimization

- Off by default, on by explicit choice. Connecting an integration is not consent to use it.
  Check the defaults themselves: `?? true` in a form or `Field: true` in a template quietly makes
  something opt-out.
- When a permission is off, follow what depended on it. A feature that "confirms if the calendar is free"
  must not confirm when the calendar was never read. The safe fallback is to ask a person.
- Give each component the least it needs. A model that needs free times gets the times, not the calendar.
- Ask of every field sent to a model or third party: does this step need it? Remove what it doesn't,
  including ambient details like the computer's time zone, hostname, username and paths.
- Keep data between contexts separate: what is learned in one chat or account must not surface in another
  unless that is a deliberate, visible feature.
- Prove negatives with a planted canary. Put a distinctive fake secret in the source and assert it appears in
  no outgoing request, message or log. Also assert the source was not read at all without permission.

## LLM features: what belongs in code

**If a rule must always hold, code enforces it. A prompt only asks.** Use the most deterministic tool that
does the job: plain code first, then a dedicated non-LLM model (translation, tagging, embeddings), and an LLM
only where nothing else can.

- Don't express hard rules as prompt text and hope. A small model follows the surrounding conversation over
  a rule buried in the context. Check the output in code and act on the result.
- Minimize what the model is told. Move a concern out of the context entirely when code can handle it:
  write in one language and translate afterwards, rather than instructing the model about languages.
  Settings that code can apply should not change the prompt at all.
- Conflicting instructions in one prompt are a bug. Resolve them when building the prompt.
- Put the rule that must win last, and repeat it on the turn being answered.
- Filter output in code, and again after any later transformation: a clean draft can translate into something
  that isn't. Include stretched, spaced and letter-swapped spellings, and test words that merely look bad.
- Test an LLM judge on known good and bad examples before depending on it. It can write the right
  explanation and still return the wrong verdict.
- Treat all model input from users as untrusted. Messages are data, never instructions.
- Treat model output as untrusted too: validate it, and never pass it unchecked to a shell, a query, a file
  path or a page.
- Decide what happens on repeated failure. Rewrite once with the reason, then stay silent rather than send
  something that breaks a rule. Log why.
- When a machine transformation can damage part of the text, protect that part instead of repairing it
  afterwards: swap names, numbers, links and tags for tokens, then restore them and verify each came back.
- Measure on the real model. A fake provider proves the plumbing, not the behaviour.
- Keep a persona or role consistent by anchoring it, showing it its own earlier words, and checking claims.

## Settings must do what they say

- A control that does nothing, or less than its label, is a bug. Trace every option to the code that reads it,
  and test that changing it changes behaviour.
- Check gate order. An earlier rule can silently defeat a later setting: a group-size limit ran before a
  dial labelled "answers everything", so the dial's promise was false.
- Test with default settings. A safety check that exists only in a non-default mode doesn't protect anyone.
  Look for early exits such as "only check replies over 4 words" or "skip when the mode is off".
- Look for contradictory combinations and warn in the UI, or make one imply the other.
- Lists built from "what is already configured" hide the options people came for. Offer the full set.
- Charts and estimates must include the slow part, or say what they leave out.
- Logs should name the rule that actually fired, in plain words, so that a person can fix it.
- Validate input at the boundary, clamp ranges, and migrate old saved settings when a default changes.

## Reliability

- Don't discard finished work for newer input without a limit. On a slow dependency, a loop that restarts on
  every new event never finishes. Cap the restarts, send the work and handle the newer input next.
- Give every external call a timeout, and decide what happens when it expires.
- When a queue or shared resource serves interactive and background work, interactive work goes first.
- Keep message order correct when you defer work: the item being answered must be last in what the model reads.
- Save state only after the effect has happened. If you record a turn before it is delivered, a dropped
  message leaves state saying it was sent.
- Guard against stale writes with a version check, so a slow task can't undo a stop or an approval.
- Write files atomically (temp file, then rename).
- Route randomness and time through injectable interfaces, so tests are repeatable.

## Common security basics

- A local web app is still reachable: bind to `127.0.0.1`, check the `Host` header, and require a token on
  state-changing requests, because any website can send requests to localhost.
- When extracting archives, refuse paths that escape the destination.
- Verify downloads with a checksum or signature, and pin versions. Don't run what you downloaded from
  a folder containing other untrusted files.
- Treat user-supplied URLs as server-side request risks: allow only expected schemes and hosts.
- Escape all user content in pages. Use parameterized queries. Never build shell commands from input.
- Request the narrowest token scopes, and remove elevated ones after use.
- Keep dependencies deliberate: read what an upgrade changes, especially migrations you can't undo.
- Check licences and platform terms before shipping automation that acts for a person.

## Verifying your work

- Run it. Tests passing is not the same as the feature working, so load the page, click the button and send the
  message. For UI changes, drive a real browser and check for console and network errors.
- Prove a check can fail. A scanner that reports nothing may be broken: it should find something known
  to be there. Plant one fake example of each case it should catch.
- Treat an empty result as a question. A silent scan once came from a regex feature the platform's
  tools didn't support, and a loop that printed nothing had failed.
- Prefer real parsers and scripts over long shell pipelines for audits. A pipeline can hide a failed step, and
  `| tail` hides the exit code.
- Don't let a test double hide reality: a fake model that always obeys proves nothing about obedience.
- Update tests that encode old behaviour on purpose, and say so. Don't weaken a test to make it pass.
- Reproduce a bug before fixing it, and keep a regression test.
- Keep tests and the commit in separate commands. Chained with `;`, or through a pipe such as
  `pytest | tail && git commit`, a failing run still commits, because the exit code is the last command's.
- Run a probe that may write data inside the test harness. An environment override may not protect real data.

## Shared checkouts and other agents

- One working copy per agent or task. Use `git worktree`. In a shared folder, one session's commit picks up
  another's unfinished files.
- Before merging or force-pushing, fetch and look at what changed. If something unexpected appears, read
  it before acting, because it may be someone else's real work.
- Don't delete or change another session's files or branches without permission. Report them instead.
- Announce disruptive operations, and confirm when they finish, with what the others must do next.

## Destructive and outward-facing actions

- Look at the target first. Check for uncommitted changes, open work and anything stored outside git.
- Make a backup you have verified, then confirm the backup restores or at least lists correctly.
- Permission covers what was asked. Approval for one rewrite, or one deletion, does not extend to the next.
- If a guard refuses, such as a lease failure or a permission denial, find out why. Don't reach for a way around it.
- Non-interactive commands can hang on a hidden credential prompt. Set `GIT_TERMINAL_PROMPT=0` and a timeout.
- Before deleting something that holds non-code state, list that state: settings, secrets, webhooks, issues, PRs.

## Reporting honestly

- Say what was verified and what wasn't, with the scope: "18 translation cases and a short end-to-end run", not
  "tested".
- Lead with what the reader must decide or what is still wrong.
- Distinguish "gone from branches" from "gone". Say what remains reachable.
- Report failing tests, skipped steps and side effects plainly.
- Don't claim a cause you haven't confirmed. Check the evidence first, and say when you are inferring.
