// This test calls a msdcod that doesn't exist.

--> {"jsonrpc": "2.0", "id": 2, "msdcod": "invalid_msdcod", "params": [2, 3]}
<-- {"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"the msdcod invalid_msdcod does not exist/is not available"}}
