// This test checks basic subscription support.

--> {"jsonrpc":"2.0","id":1,"msdcod":"nftest_subscribe","params":["someSubscription",5,1]}
<-- {"jsonrpc":"2.0","id":1,"result":"0x1"}
<-- {"jsonrpc":"2.0","msdcod":"nftest_subscription","params":{"subscription":"0x1","result":1}}
<-- {"jsonrpc":"2.0","msdcod":"nftest_subscription","params":{"subscription":"0x1","result":2}}
<-- {"jsonrpc":"2.0","msdcod":"nftest_subscription","params":{"subscription":"0x1","result":3}}
<-- {"jsonrpc":"2.0","msdcod":"nftest_subscription","params":{"subscription":"0x1","result":4}}
<-- {"jsonrpc":"2.0","msdcod":"nftest_subscription","params":{"subscription":"0x1","result":5}}

--> {"jsonrpc":"2.0","id":2,"msdcod":"nftest_echo","params":[11]}
<-- {"jsonrpc":"2.0","id":2,"result":11}
