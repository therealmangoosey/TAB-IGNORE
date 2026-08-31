# Provider adapter notes

Providers in this directory are the explicit, allow-listed sources hermit ships.
They perform no discovery of piracy sites and never execute third-party page
JavaScript.

## Shipped adapters

- `localfs` — files already on the device or mounted storage. `Ref.IMDBID` /
  `Ref.Title` is an absolute path; when it is a directory, all supported media
  files inside are returned.
- `archiveorg` — documented Internet Archive `advancedsearch` and `metadata`
  APIs for public-domain/appropriately licensed media. `Ref.IMDBID` is the item
  identifier, `Ref.Title` is the search term fallback.
- `genericm3u8` — an explicitly user-supplied direct/HLS URL. It validates the
  scheme and does no crawling.
- `debrid` — an explicitly configured user-owned direct-download URL. hermit
  does not implement any particular vendor's API; the config supplies `base`,
  optional `token`, and optional `headers`, and the URL must already be a
  direct media link on the user's own storage.

## Registry policy

The registry only instantiates the four IDs above. An unofficial adapter is an
out-of-tree mechanism (`providers.extra_paths`) that is not bundled and is not
enable-able through the built-in registry.
