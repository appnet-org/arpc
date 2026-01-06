@0x8c0c0c0c0c0c0c0c;

using Go = import "/go.capnp";

$Go.package("money_capnp");
$Go.import("github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid/test/money_capnp");

# Represents an amount of money with its currency type.
struct Money {
    # The 3-letter currency code defined in ISO 4217.
    currencyCode @0 :Text;

    # The whole units of the amount.
    # For example if `currencyCode` is `"USD"`, then 1 unit is one US dollar.
    units @1 :Int64;

    # Number of nano (10^-9) units of the amount.
    # The value must be between -999,999,999 and +999,999,999 inclusive.
    # If `units` is positive, `nanos` must be positive or zero.
    # If `units` is zero, `nanos` can be positive, zero, or negative.
    # If `units` is negative, `nanos` must be negative or zero.
    # For example $-1.75 is represented as `units`=-1 and `nanos`=-750,000,000.
    nanos @2 :Int32;
}

struct CurrencyConversionRequest {
    from @0 :Money;

    # The 3-letter currency code defined in ISO 4217.
    toCode @1 :Text;

    userId @2 :Text;
}

