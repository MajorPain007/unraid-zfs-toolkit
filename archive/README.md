# archive/

Holds only the package the current `.plg` points at. Older builds were removed;
they were only reachable by a `.plg` nobody runs any more, and deleting them
from the working tree does not shrink a clone anyway - the blobs stay in the
git history either way.

Do not add new `.txz` files by hand. Tag a version and the release workflow
builds the package, attaches it to a GitHub Release and rewrites `pkgURL` in
`src/zfs.dataset.converter.plg` to point there:

```bash
git tag 2026.08.08.08 && git push origin 2026.08.08.08
```

From that first tagged release on, this directory stops growing.
