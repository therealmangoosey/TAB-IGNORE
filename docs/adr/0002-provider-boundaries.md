# ADR 0002: Provider boundaries

Status: accepted

## Context
The original plan surveyed several aggregator sites that are unauthorized
redistribution, use JavaScript/embed cascades, and change domains frequently.
Bundling adapters for them would create a maintenance and legal liability and
would require executing remote JS, which is explicitly unsafe.

## Decision
- The engine is source-agnostic and complete against the `Provider` interface.
- In-tree providers are limited to local files, Internet Archive public-domain
  media, user-supplied generic media URLs, and user-owned direct-download links.
- Unofficial adapters live out of tree behind `providers.extra_paths`.
- The registry refuses unknown/unofficial provider IDs.
- Provider base URLs are config data, never hardcoded.

## Consequences
- Domain churn is a config edit, not a release.
- The download/queue/stream engine remains testable with fake transports.
- No scraper, CAPTCHA bypass, Cloudflare bypass, or DRM circumvention code is
  present.
