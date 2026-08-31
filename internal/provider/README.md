# Provider adapter notes

This directory is the extension point for media source providers.

Adapters must be deterministic, testable, context-aware, and safe by default. Network access should be performed by the fetch/transport layer, not by arbitrary HTTP calls scattered across adapters.

Initial supported categories:

- `localfs`: files already on the device or mounted storage.
- `archiveorg`: documented Internet Archive endpoints for public-domain/appropriately licensed media.
- `genericm3u8`: a media manifest URL explicitly supplied by the user.
- `debrid`: an explicitly configured user-owned service, once its API contract is defined.

For each new adapter, document its accepted input, output, authentication method, rate limits, error states, and legal/usage assumptions. Add fixtures and tests before wiring it into the registry.
