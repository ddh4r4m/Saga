# Error codes

Every code cartctl can print, in the format `error <code>: <message>`.

| code | message | when |
|---|---|---|
| E_UNKNOWN_SKU | unknown sku | the sku is not in the catalogue |
| E_EMPTY_CART | cart is empty | checkout was asked for an empty cart |
| E_USAGE | usage: cartctl add <sku> <qty> | the command line could not be parsed |
