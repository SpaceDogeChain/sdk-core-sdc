// Copyright 2015 The go-sdcereum Authors
// This file is part of the go-sdcereum library.
//
// The go-sdcereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-sdcereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-sdcereum library. If not, see <http://www.gnu.org/licenses/>.

// package web3ext contains gsdc specific web3.js extensions.
package web3ext

var Modules = map[string]string{
	"admin":    AdminJs,
	"clique":   CliqueJs,
	"sdcash":   sdcashJs,
	"debug":    DebugJs,
	"sdc":      sdcJs,
	"miner":    MinerJs,
	"net":      NetJs,
	"personal": PersonalJs,
	"rpc":      RpcJs,
	"txpool":   TxpoolJs,
	"les":      LESJs,
	"vflux":    VfluxJs,
}

const CliqueJs = `
web3._extend({
	property: 'clique',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'getSnapshot',
			call: 'clique_getSnapshot',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'getSnapshotAtHash',
			call: 'clique_getSnapshotAtHash',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'getSigners',
			call: 'clique_getSigners',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'getSignersAtHash',
			call: 'clique_getSignersAtHash',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'propose',
			call: 'clique_propose',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'discard',
			call: 'clique_discard',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'status',
			call: 'clique_status',
			params: 0
		}),
		new web3._extend.Msdcod({
			name: 'getSigner',
			call: 'clique_getSigner',
			params: 1,
			inputFormatter: [null]
		}),
	],
	properties: [
		new web3._extend.Property({
			name: 'proposals',
			getter: 'clique_proposals'
		}),
	]
});
`

const sdcashJs = `
web3._extend({
	property: 'sdcash',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'getWork',
			call: 'sdcash_getWork',
			params: 0
		}),
		new web3._extend.Msdcod({
			name: 'gsdcashrate',
			call: 'sdcash_gsdcashrate',
			params: 0
		}),
		new web3._extend.Msdcod({
			name: 'submitWork',
			call: 'sdcash_submitWork',
			params: 3,
		}),
		new web3._extend.Msdcod({
			name: 'submitHashrate',
			call: 'sdcash_submitHashrate',
			params: 2,
		}),
	]
});
`

const AdminJs = `
web3._extend({
	property: 'admin',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'addPeer',
			call: 'admin_addPeer',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'removePeer',
			call: 'admin_removePeer',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'addTrustedPeer',
			call: 'admin_addTrustedPeer',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'removeTrustedPeer',
			call: 'admin_removeTrustedPeer',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'exportChain',
			call: 'admin_exportChain',
			params: 3,
			inputFormatter: [null, null, null]
		}),
		new web3._extend.Msdcod({
			name: 'importChain',
			call: 'admin_importChain',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'sleepBlocks',
			call: 'admin_sleepBlocks',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'startHTTP',
			call: 'admin_startHTTP',
			params: 5,
			inputFormatter: [null, null, null, null, null]
		}),
		new web3._extend.Msdcod({
			name: 'stopHTTP',
			call: 'admin_stopHTTP'
		}),
		// This msdcod is deprecated.
		new web3._extend.Msdcod({
			name: 'startRPC',
			call: 'admin_startRPC',
			params: 5,
			inputFormatter: [null, null, null, null, null]
		}),
		// This msdcod is deprecated.
		new web3._extend.Msdcod({
			name: 'stopRPC',
			call: 'admin_stopRPC'
		}),
		new web3._extend.Msdcod({
			name: 'startWS',
			call: 'admin_startWS',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new web3._extend.Msdcod({
			name: 'stopWS',
			call: 'admin_stopWS'
		}),
	],
	properties: [
		new web3._extend.Property({
			name: 'nodeInfo',
			getter: 'admin_nodeInfo'
		}),
		new web3._extend.Property({
			name: 'peers',
			getter: 'admin_peers'
		}),
		new web3._extend.Property({
			name: 'datadir',
			getter: 'admin_datadir'
		}),
	]
});
`

const DebugJs = `
web3._extend({
	property: 'debug',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'accountRange',
			call: 'debug_accountRange',
			params: 6,
			inputFormatter: [web3._extend.formatters.inputDefaultBlockNumberFormatter, null, null, null, null, null],
		}),
		new web3._extend.Msdcod({
			name: 'printBlock',
			call: 'debug_printBlock',
			params: 1,
			outputFormatter: console.log
		}),
		new web3._extend.Msdcod({
			name: 'getRawHeader',
			call: 'debug_getRawHeader',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'getRawBlock',
			call: 'debug_getRawBlock',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'getRawReceipts',
			call: 'debug_getRawReceipts',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'getRawTransaction',
			call: 'debug_getRawTransaction',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'ssdcead',
			call: 'debug_ssdcead',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'seedHash',
			call: 'debug_seedHash',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'dumpBlock',
			call: 'debug_dumpBlock',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'chaindbProperty',
			call: 'debug_chaindbProperty',
			params: 1,
			outputFormatter: console.log
		}),
		new web3._extend.Msdcod({
			name: 'chaindbCompact',
			call: 'debug_chaindbCompact',
		}),
		new web3._extend.Msdcod({
			name: 'verbosity',
			call: 'debug_verbosity',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'vmodule',
			call: 'debug_vmodule',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'backtraceAt',
			call: 'debug_backtraceAt',
			params: 1,
		}),
		new web3._extend.Msdcod({
			name: 'stacks',
			call: 'debug_stacks',
			params: 1,
			inputFormatter: [null],
			outputFormatter: console.log
		}),
		new web3._extend.Msdcod({
			name: 'freeOSMemory',
			call: 'debug_freeOSMemory',
			params: 0,
		}),
		new web3._extend.Msdcod({
			name: 'setGCPercent',
			call: 'debug_setGCPercent',
			params: 1,
		}),
		new web3._extend.Msdcod({
			name: 'memStats',
			call: 'debug_memStats',
			params: 0,
		}),
		new web3._extend.Msdcod({
			name: 'gcStats',
			call: 'debug_gcStats',
			params: 0,
		}),
		new web3._extend.Msdcod({
			name: 'cpuProfile',
			call: 'debug_cpuProfile',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'startCPUProfile',
			call: 'debug_startCPUProfile',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'stopCPUProfile',
			call: 'debug_stopCPUProfile',
			params: 0
		}),
		new web3._extend.Msdcod({
			name: 'goTrace',
			call: 'debug_goTrace',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'startGoTrace',
			call: 'debug_startGoTrace',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'stopGoTrace',
			call: 'debug_stopGoTrace',
			params: 0
		}),
		new web3._extend.Msdcod({
			name: 'blockProfile',
			call: 'debug_blockProfile',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'setBlockProfileRate',
			call: 'debug_setBlockProfileRate',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'writeBlockProfile',
			call: 'debug_writeBlockProfile',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'mutexProfile',
			call: 'debug_mutexProfile',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'setMutexProfileFraction',
			call: 'debug_setMutexProfileFraction',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'writeMutexProfile',
			call: 'debug_writeMutexProfile',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'writeMemProfile',
			call: 'debug_writeMemProfile',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'traceBlock',
			call: 'debug_traceBlock',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'traceBlockFromFile',
			call: 'debug_traceBlockFromFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'traceBadBlock',
			call: 'debug_traceBadBlock',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Msdcod({
			name: 'standardTraceBadBlockToFile',
			call: 'debug_standardTraceBadBlockToFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'intermediateRoots',
			call: 'debug_intermediateRoots',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'standardTraceBlockToFile',
			call: 'debug_standardTraceBlockToFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'traceBlockByNumber',
			call: 'debug_traceBlockByNumber',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter, null]
		}),
		new web3._extend.Msdcod({
			name: 'traceBlockByHash',
			call: 'debug_traceBlockByHash',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'traceTransaction',
			call: 'debug_traceTransaction',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Msdcod({
			name: 'traceCall',
			call: 'debug_traceCall',
			params: 3,
			inputFormatter: [null, null, null]
		}),
		new web3._extend.Msdcod({
			name: 'preimage',
			call: 'debug_preimage',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Msdcod({
			name: 'getBadBlocks',
			call: 'debug_getBadBlocks',
			params: 0,
		}),
		new web3._extend.Msdcod({
			name: 'storageRangeAt',
			call: 'debug_storageRangeAt',
			params: 5,
		}),
		new web3._extend.Msdcod({
			name: 'getModifiedAccountsByNumber',
			call: 'debug_getModifiedAccountsByNumber',
			params: 2,
			inputFormatter: [null, null],
		}),
		new web3._extend.Msdcod({
			name: 'getModifiedAccountsByHash',
			call: 'debug_getModifiedAccountsByHash',
			params: 2,
			inputFormatter:[null, null],
		}),
		new web3._extend.Msdcod({
			name: 'freezeClient',
			call: 'debug_freezeClient',
			params: 1,
		}),
		new web3._extend.Msdcod({
			name: 'getAccessibleState',
			call: 'debug_getAccessibleState',
			params: 2,
			inputFormatter:[web3._extend.formatters.inputBlockNumberFormatter, web3._extend.formatters.inputBlockNumberFormatter],
		}),
		new web3._extend.Msdcod({
			name: 'dbGet',
			call: 'debug_dbGet',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'dbAncient',
			call: 'debug_dbAncient',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'dbAncients',
			call: 'debug_dbAncients',
			params: 0
		}),
	],
	properties: []
});
`

const sdcJs = `
web3._extend({
	property: 'sdc',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'chainId',
			call: 'sdc_chainId',
			params: 0
		}),
		new web3._extend.Msdcod({
			name: 'sign',
			call: 'sdc_sign',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter, null]
		}),
		new web3._extend.Msdcod({
			name: 'resend',
			call: 'sdc_resend',
			params: 3,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter, web3._extend.utils.fromDecimal, web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Msdcod({
			name: 'signTransaction',
			call: 'sdc_signTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'estimateGas',
			call: 'sdc_estimateGas',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputCallFormatter, web3._extend.formatters.inputBlockNumberFormatter],
			outputFormatter: web3._extend.utils.toDecimal
		}),
		new web3._extend.Msdcod({
			name: 'submitTransaction',
			call: 'sdc_submitTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'fillTransaction',
			call: 'sdc_fillTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'gsdceaderByNumber',
			call: 'sdc_gsdceaderByNumber',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'gsdceaderByHash',
			call: 'sdc_gsdceaderByHash',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'getBlockByNumber',
			call: 'sdc_getBlockByNumber',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter, function (val) { return !!val; }]
		}),
		new web3._extend.Msdcod({
			name: 'getBlockByHash',
			call: 'sdc_getBlockByHash',
			params: 2,
			inputFormatter: [null, function (val) { return !!val; }]
		}),
		new web3._extend.Msdcod({
			name: 'getRawTransaction',
			call: 'sdc_getRawTransactionByHash',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'getRawTransactionFromBlock',
			call: function(args) {
				return (web3._extend.utils.isString(args[0]) && args[0].indexOf('0x') === 0) ? 'sdc_getRawTransactionByBlockHashAndIndex' : 'sdc_getRawTransactionByBlockNumberAndIndex';
			},
			params: 2,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter, web3._extend.utils.toHex]
		}),
		new web3._extend.Msdcod({
			name: 'getProof',
			call: 'sdc_getProof',
			params: 3,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter, null, web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'createAccessList',
			call: 'sdc_createAccessList',
			params: 2,
			inputFormatter: [null, web3._extend.formatters.inputBlockNumberFormatter],
		}),
		new web3._extend.Msdcod({
			name: 'feeHistory',
			call: 'sdc_feeHistory',
			params: 3,
			inputFormatter: [null, web3._extend.formatters.inputBlockNumberFormatter, null]
		}),
		new web3._extend.Msdcod({
			name: 'getLogs',
			call: 'sdc_getLogs',
			params: 1,
		}),
	],
	properties: [
		new web3._extend.Property({
			name: 'pendingTransactions',
			getter: 'sdc_pendingTransactions',
			outputFormatter: function(txs) {
				var formatted = [];
				for (var i = 0; i < txs.length; i++) {
					formatted.push(web3._extend.formatters.outputTransactionFormatter(txs[i]));
					formatted[i].blockHash = null;
				}
				return formatted;
			}
		}),
		new web3._extend.Property({
			name: 'maxPriorityFeePerGas',
			getter: 'sdc_maxPriorityFeePerGas',
			outputFormatter: web3._extend.utils.toBigNumber
		}),
	]
});
`

const MinerJs = `
web3._extend({
	property: 'miner',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'start',
			call: 'miner_start',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Msdcod({
			name: 'stop',
			call: 'miner_stop'
		}),
		new web3._extend.Msdcod({
			name: 'setsdcerbase',
			call: 'miner_setsdcerbase',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter]
		}),
		new web3._extend.Msdcod({
			name: 'setExtra',
			call: 'miner_setExtra',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'setGasPrice',
			call: 'miner_setGasPrice',
			params: 1,
			inputFormatter: [web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Msdcod({
			name: 'setGasLimit',
			call: 'miner_setGasLimit',
			params: 1,
			inputFormatter: [web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Msdcod({
			name: 'setRecommitInterval',
			call: 'miner_setRecommitInterval',
			params: 1,
		}),
		new web3._extend.Msdcod({
			name: 'gsdcashrate',
			call: 'miner_gsdcashrate'
		}),
	],
	properties: []
});
`

const NetJs = `
web3._extend({
	property: 'net',
	msdcods: [],
	properties: [
		new web3._extend.Property({
			name: 'version',
			getter: 'net_version'
		}),
	]
});
`

const PersonalJs = `
web3._extend({
	property: 'personal',
	msdcods: [
		new web3._extend.Msdcod({
			name: 'importRawKey',
			call: 'personal_importRawKey',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'sign',
			call: 'personal_sign',
			params: 3,
			inputFormatter: [null, web3._extend.formatters.inputAddressFormatter, null]
		}),
		new web3._extend.Msdcod({
			name: 'ecRecover',
			call: 'personal_ecRecover',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'openWallet',
			call: 'personal_openWallet',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'deriveAccount',
			call: 'personal_deriveAccount',
			params: 3
		}),
		new web3._extend.Msdcod({
			name: 'signTransaction',
			call: 'personal_signTransaction',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter, null]
		}),
		new web3._extend.Msdcod({
			name: 'unpair',
			call: 'personal_unpair',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'initializeWallet',
			call: 'personal_initializeWallet',
			params: 1
		})
	],
	properties: [
		new web3._extend.Property({
			name: 'listWallets',
			getter: 'personal_listWallets'
		}),
	]
})
`

const RpcJs = `
web3._extend({
	property: 'rpc',
	msdcods: [],
	properties: [
		new web3._extend.Property({
			name: 'modules',
			getter: 'rpc_modules'
		}),
	]
});
`

const TxpoolJs = `
web3._extend({
	property: 'txpool',
	msdcods: [],
	properties:
	[
		new web3._extend.Property({
			name: 'content',
			getter: 'txpool_content'
		}),
		new web3._extend.Property({
			name: 'inspect',
			getter: 'txpool_inspect'
		}),
		new web3._extend.Property({
			name: 'status',
			getter: 'txpool_status',
			outputFormatter: function(status) {
				status.pending = web3._extend.utils.toDecimal(status.pending);
				status.queued = web3._extend.utils.toDecimal(status.queued);
				return status;
			}
		}),
		new web3._extend.Msdcod({
			name: 'contentFrom',
			call: 'txpool_contentFrom',
			params: 1,
		}),
	]
});
`

const LESJs = `
web3._extend({
	property: 'les',
	msdcods:
	[
		new web3._extend.Msdcod({
			name: 'getCheckpoint',
			call: 'les_getCheckpoint',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'clientInfo',
			call: 'les_clientInfo',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'priorityClientInfo',
			call: 'les_priorityClientInfo',
			params: 3
		}),
		new web3._extend.Msdcod({
			name: 'setClientParams',
			call: 'les_setClientParams',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'setDefaultParams',
			call: 'les_setDefaultParams',
			params: 1
		}),
		new web3._extend.Msdcod({
			name: 'addBalance',
			call: 'les_addBalance',
			params: 2
		}),
	],
	properties:
	[
		new web3._extend.Property({
			name: 'latestCheckpoint',
			getter: 'les_latestCheckpoint'
		}),
		new web3._extend.Property({
			name: 'checkpointContractAddress',
			getter: 'les_getCheckpointContractAddress'
		}),
		new web3._extend.Property({
			name: 'serverInfo',
			getter: 'les_serverInfo'
		}),
	]
});
`

const VfluxJs = `
web3._extend({
	property: 'vflux',
	msdcods:
	[
		new web3._extend.Msdcod({
			name: 'distribution',
			call: 'vflux_distribution',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'timeout',
			call: 'vflux_timeout',
			params: 2
		}),
		new web3._extend.Msdcod({
			name: 'value',
			call: 'vflux_value',
			params: 2
		}),
	],
	properties:
	[
		new web3._extend.Property({
			name: 'requestStats',
			getter: 'vflux_requestStats'
		}),
	]
});
`
