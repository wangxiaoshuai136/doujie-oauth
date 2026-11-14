# Release Checklist

Every release must include a changelog written for users, not just a commit list.

1. Pick the next version and update it in:
   - `go/dispatch.go`
   - `Makefile`
   - `.github/workflows/build.yml`
2. Add a new section at the top of `CHANGELOG.md`:

   ```markdown
   ## 0.2.4 — 2026-11-15

   ### 安全
   - What changed.
   - Why it matters to users.
   - Any action operators must take.
   ```

   Write it so a user who does not read commits understands:
   - what changed;
   - whether behavior changed;
   - whether they need to upgrade, reconfigure, or re-login.

3. Do not rely on GitHub's compare link as release notes. The release workflow
   extracts the matching `## <version>` section from `CHANGELOG.md` and uses it
   as the release body.
4. The release job fails if the section is missing or too short. Fix the
   changelog before tagging again.
5. Push the tag `vX.Y.Z`; the workflow builds all platform archives, verifies
   assets, and publishes the release.
6. If the plugin is listed in the CPA plugin store, update the registry entry to
   the new version and request a fresh review.
