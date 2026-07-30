package web3ext

// TkmAccountJs exposes the non-consensus smart-account calldata and hash helpers.
const TkmAccountJs = `
web3._extend({
	property: 'tkmaccount',
	methods: [
		new web3._extend.Method({name: 'status', call: 'tkmaccount_status', params: 0}),
		new web3._extend.Method({name: 'operationHash', call: 'tkmaccount_operationHash', params: 3}),
		new web3._extend.Method({name: 'createData', call: 'tkmaccount_createData', params: 1}),
		new web3._extend.Method({name: 'executeData', call: 'tkmaccount_executeData', params: 3}),
		new web3._extend.Method({name: 'setOwnersData', call: 'tkmaccount_setOwnersData', params: 2}),
		new web3._extend.Method({name: 'setLimitsData', call: 'tkmaccount_setLimitsData', params: 2}),
		new web3._extend.Method({name: 'setGuardianData', call: 'tkmaccount_setGuardianData', params: 2}),
		new web3._extend.Method({name: 'setRecoveryPolicyData', call: 'tkmaccount_setRecoveryPolicyData', params: 2}),
		new web3._extend.Method({name: 'recoveryHash', call: 'tkmaccount_recoveryHash', params: 2}),
		new web3._extend.Method({name: 'approveRecoveryData', call: 'tkmaccount_approveRecoveryData', params: 1}),
		new web3._extend.Method({name: 'cancelRecoveryData', call: 'tkmaccount_cancelRecoveryData', params: 0}),
		new web3._extend.Method({name: 'completeRecoveryData', call: 'tkmaccount_completeRecoveryData', params: 2}),
		new web3._extend.Method({name: 'setSessionData', call: 'tkmaccount_setSessionData', params: 1}),
		new web3._extend.Method({name: 'revokeSessionData', call: 'tkmaccount_revokeSessionData', params: 1}),
		new web3._extend.Method({name: 'ownerAuthorization', call: 'tkmaccount_ownerAuthorization', params: 1}),
		new web3._extend.Method({name: 'sessionAuthorization', call: 'tkmaccount_sessionAuthorization', params: 1}),
		new web3._extend.Method({name: 'sponsorshipHash', call: 'tkmaccount_sponsorshipHash', params: 4}),
		new web3._extend.Method({name: 'sponsorshipData', call: 'tkmaccount_sponsorshipData', params: 2}),
		new web3._extend.Method({name: 'predictAddress', call: 'tkmaccount_predictAddress', params: 3})
	]
});
`
