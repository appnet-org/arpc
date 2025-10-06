# Cap’n Proto aRPC Compiler

**Step 1: Install Cap'n Proto Tools**

```bash
curl -O https://capnproto.org/capnproto-c++-1.1.0.tar.gz
tar zxf capnproto-c++-1.1.0.tar.gz
cd capnproto-c++-1.1.0
./configure
make -j6 check
sudo make install
```

**Step 2: Install Capnp Go Plugin**

```bash
go install capnproto.org/go/capnp/v3/capnpc-go@v3.1.0-alpha.1
```

**Step 3: Generate aRPC Stubs**

In `cmd/capnp-gen-arpc`, run

```bash
./capnpc.sh <path-to-capnp-file>
# Example: ./capnpc.sh ../../examples/echo_capnp/capnp/echo.capnp
```

Make sure `$GOPATH/bin` is in your shell `PATH`.