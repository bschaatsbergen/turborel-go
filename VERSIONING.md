# Versions

turborel-go is unreleased alpha software. No stable Go API or server
compatibility across unrelated alpha builds is promised.

The client speaks native protocol version 1 and is tested against
[Turborel 3f25a033dad94814fbd4e7d871e48eb81b2734cc](https://github.com/turborel/turborel/tree/3f25a033dad94814fbd4e7d871e48eb81b2734cc).
CI checks out that exact revision. The module requires Go 1.27; the
verification toolchain is Go 1.27.4.

Before changing the server pin, compare protocol/src and client/src,
rerun the complete verification command and update CONFORMANCE.md for
any changed behavior. A future release uses a Go module version tag;
no release has been made by this initial implementation.
