# Changelog

[Historique détaillé en français](CHANGELOG.fr.md)

## 0.2.94

- Cron tasks started on demand now inherit the target Unix account's
  supplementary groups, allowing legitimate group-based access such as an
  `adm` member reading Apache logs.

## 0.2.93

- Align the contents of the five Cron schedule cards to the top and give their
  clear buttons one compact, consistent size.

## 0.2.92

- The Cron task dialog uses a compact five-column schedule grid on large
  screens, with responsive fallback and scrolling confined to long value lists.

## 0.2.91

- Cron task creation and editing use a single editable expression field
  synchronized with the schedule checkboxes; the mode selector and duplicate
  generated-expression preview were removed.
- Expressions that cannot be represented by checkboxes remain unchanged while
  the checkboxes are disabled until the expression becomes representable.

## 0.2.90

- The Logs backend reports the total number of lines in the selected file,
  independently of the requested window and active filters.
- The result summary now compares displayed lines with that complete file
  total rather than with the user-selected request size.

## 0.2.89

- An immediate reboot requested from Updates is now scheduled five seconds
  later, allowing the HTTP confirmation page to reach the browser before the
  services stop.
- A dedicated bilingual waiting screen detects the interruption, retries the
  health endpoint and reloads AegisAdmin as soon as the server is available.

## 0.2.88

- Resetting the Logs search now clears every filter and returns to the initial
  page state, including the configured default log and the 100-line window.

## 0.2.87

- The Logs screen accepts a user-defined window from 1 to 5,000 lines instead
  of always requesting the latest 100 entries.
- Its result summary now reports the selected window.

## 0.2.86

- The standalone download dashboard now emits empty JSON collections as arrays
  and tolerates older statistics files containing `null`, so a repository with
  no recorded package download displays zero instead of a JavaScript error.

## 0.2.85

- A standalone private dashboard aggregates successful APT package downloads
  by day, version and architecture without copying client IP addresses.
- Public GitHub Release asset counters are merged when available; GitHub API
  failures do not prevent publication of APT statistics.
- A hardened systemd timer and password-protected Apache configuration provide
  hourly generation without installing AegisAdmin on the package server.
- The Updates interface consistently uses the English label “Details”.

## 0.2.84

- English is now the default language of the GitHub README, contribution
  guide, security policy and documentation index.
- A complete English documentation tree is available under `docs/en`, while
  the maintained French documents remain available through a dedicated index.
- English and French documentation are both included in the Debian package.
- Future release notes and public-facing GitHub documentation will be written
  in English first and kept synchronized with French translations.

## 0.2.83

- Installation, operations and testing documentation now distinguish direct Go
  HTTPS from the optional Apache reverse proxy.
- Service names, ports, persistent paths, initialization, language selection
  and optional MySQL setup were aligned with the current package.
- A modules and permissions guide documents None, View, Actions and Modify.

## 0.2.82

- Dynamic Linux process states are translated according to the profile locale
  while their technical state codes remain visible.

## 0.2.81

- The About page became fully bilingual.
- English sign-out actions were standardized on “Sign out”.

## Previous 0.2.x candidates

The detailed history of candidates 0.2.0 through 0.2.80 is preserved in
[CHANGELOG.fr.md](CHANGELOG.fr.md). It covers the Go migration, Debian package,
signed APT repository, permissions, TOTP, updates, Composer monitoring,
configuration snapshots, themes and incremental French/English interface
translation.
