# Open work

- Expose affected-row counts when the server adds them to completion messages.
- Expose generated keys when the server provides them.
- Add Windows named-pipe transport; TCP with TLS already works on Windows.
- Provide a richer native result API for opaque extension payloads and
  typed composite scanning, beyond database/sql's scalar value contract.
- Add fixtures for TLS 1.2 channel binding and live Unix-socket connections;
  TLS 1.3 and TCP are covered end to end.
- Add compatibility jobs for future released server versions once available.
