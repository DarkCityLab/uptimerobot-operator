<!--
Thanks for contributing! Please fill this out so review is quick.
See CONTRIBUTING.md for the full workflow.
-->

## Summary

<!-- What does this PR do, and why? -->

## Related issues

<!-- e.g. "Closes #123". Use "Closes" so the issue auto-closes on merge. -->

## Type of change

- [ ] Bug fix (`fix:`)
- [ ] New feature (`feat:`)
- [ ] Breaking change (`feat!:` / `fix!:` or `BREAKING CHANGE:` footer)
- [ ] Docs / chore / CI / refactor / test

## Checklist

- [ ] Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org).
- [ ] All commits are **signed off** for the DCO (`git commit -s`) and the sign-off matches the author. <!-- CI enforces this -->
- [ ] `make test` and `make lint` pass locally.
- [ ] I did **not** hand-edit generated files (`zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`, `PROJECT`); I ran `make manifests generate` instead.
- [ ] If I changed API types, I ran `make manifests sync-chart-crd` and updated samples/docs.
- [ ] Tests and documentation are updated as needed.

## Notes for reviewers

<!-- Anything that needs special attention, trade-offs, follow-ups, etc. -->
