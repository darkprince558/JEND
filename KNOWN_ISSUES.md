# Known Issues & Bugs

Community-maintained list of known issues. If you encounter any of these, workarounds are provided below.

---

## 🪟 Windows E2E Loopback Test Fails in CI

**Status:** Open | **Severity:** CI-only (does not affect users)

The Windows E2E loopback test (sender + receiver on the same machine) fails in GitHub Actions.

### Root Cause

Two compounding issues in the GitHub Actions Windows runner:

1. **Sender process dies between CI steps** — `Start-Process` doesn't reliably keep the sender alive across workflow steps on the Windows Server 2025 runner.
2. **Discovery returns unreachable IP** — Cloud Registry returns the runner's public IP, which can't hairpin NAT back to localhost.

### Impact

- ❌ CI loopback test on Windows
- ✅ Real Windows users (sender/receiver on different machines)
- ✅ Windows build + unit tests
- ✅ Mac/Linux E2E tests

### Workaround

Windows E2E is skipped in CI. Windows is still fully covered by build and unit tests.

### Possible Fixes

- Use Windows-native background process mechanism (`schtasks`, `nssm`)
- Test with `windows-2022` runner instead of `windows-latest`
- Use PowerShell `Start-Job` instead of `Start-Process`

---

## Contributing

Found a bug? Please [open an issue](https://github.com/darkprince558/JEND/issues/new) with:

- OS and version
- Steps to reproduce
- Expected vs actual behavior
- Any error output
