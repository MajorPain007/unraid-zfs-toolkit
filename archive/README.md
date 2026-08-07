# archive/

Historic plugin packages. **Frozen** — do not add new `.txz` files here.

Releases are published as GitHub Release assets (see
`.github/workflows/release.yml`). Tag a version and the workflow builds the
package, attaches it to the release, and rewrites `pkgURL` in
`src/zfs.dataset.converter.plg` to point at it:

```bash
git tag 2026.08.08.02 && git push origin 2026.08.08.02
```

The files here are kept so that older `.plg` manifests already installed on
someone's server can still resolve their package URL. Deleting them from the
working tree would not shrink a clone anyway — the blobs stay in the git
history either way.
