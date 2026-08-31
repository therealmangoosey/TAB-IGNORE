# ADR 0003: No traffic cloaking

Status: accepted

## Context
A user requested "hide the downloads" without a VPN because "the IP must not
change." Without a relay you cannot hide destination addresses from the network
between you and the provider while still using your own IP and path.

## Decision
- hermit does not proxy, encrypt, disguise, or hide network traffic.
- At-rest privacy is supported via `.nomedia` and metadata scrubbing; OS-level
  encryption via Secure Folder is documented.
- Doctor reports a `privacy` block and refuses to claim LAN mode when the
  server is bound to loopback with no token.

## Consequences
- If you need traffic cloaking, use a VPN/proxy or download elsewhere.
- The tool will not grow a "hide traffic" feature, which keeps the security and
  support surface small.
