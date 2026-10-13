# v6 Test & Release Report

**Release:** Windows Desktop v6 (12 October 2026)

## Implemented and tested in this Linux build environment

- Native **Windows x64 PE32+ GUI** program `SlangDiscovery.exe` generated using Go cross-compilation.
- Native **Windows x64 PE32+ GUI** installer `Setup.exe` built with the application binary embedded; intended to copy to LocalAppData and create real `.lnk` shortcuts on Desktop/Start Menu using WScript.Shell via PowerShell.
- Six automated Go tests passed: candidate extraction and human-review persistence; file persistence/backup; API request CSRF token + Origin check and saving settings; mocked Reddit OAuth/post/comment retrieval; info UI marker checks; demo ordinary-English false-positive regression plus context-drift candidate.
- Live Linux build: local HTTP app started; demo ingest and analysis successfully returned **21 synthetic documents and 25 candidates** before addition of the CMUdict dictionary. The CMUdict-enhanced variant's demo regression was covered by Go tests. Final build includes dictionary and JSONL import.
- Browser **static interface rendering** with Chromium Playwright using injected HTML/CSS/JS: main screen, settings screen, API info dialog rendered; buttons navigated; screenshot previews captured. Live browser-to-localhost requests were blocked by browser sandbox policy in this environment; not a Windows acceptance test.
- Windows DPAPI methods compiled for target but could not be executed without a Windows runtime.

## Not verified here

- Double-click execution of either Windows binary; installer prompts; creation of shortcuts on an actual Windows desktop; Windows Defender / SmartScreen behavior.
- Live Reddit OAuth with genuine credentials / data access approval, rate limits, 2027 migration compatibility.
- Real-world slang precision/recall, phrase sense correctness; large-scale ingestion performance.
- Continuous deletion compliance of Reddit posts/comments.

## Acceptance test on Windows

1. Extract zip; run `Setup.exe`; accept prompt; confirm Desktop and Start Menu links.
2. Open desktop link; localhost dashboard launches in browser; run **DEMO YÜKLE** and review `glorpmaxxing` and `cooked` candidate outputs.
3. Open **Ayarlar**; open `ⓘ REDDIT API NASIL ALINIR?`; close overlay.
4. Enter ID, Secret, custom User-Agent and save; relaunch; verify secret field stays blank with stored-state indicator. Test connection only if Reddit authorized account is available.
5. Import authorized JSONL, inspect source evidence and review status; CSV export.
6. Click window top-right ×; confirm application process exits; reopen shortcut.
7. Test repeat installation with previous app **closed**.

## Security notes

- Local server listens only on `127.0.0.1`, uses random port, and checks `Origin` and unpredictable per-run token for all POST mutations.
- Only Windows DPAPI-encrypted secret is written to file, with restrictive permissions where OS enforces them. API responses omit the secret.
- Not code-signed; zip/exe integrity checks and Windows reputation warning possible.
- No API bypass or forbidden scraping behavior added.
