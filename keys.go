package aifvalidate

// YAML key names read from a flow document. Named rather than inlined so a
// schema rename is a one-line change and a typo is a compile error, not a rule
// that silently stops firing.
const (
	keyAigentflowVersion   = "aigentflow_version"
	keyName                = "name"
	keyStart               = "start"
	keySteps               = "steps"
	keyQuery               = "query"
	keyExecutor            = "executor"
	keyNext                = "next"
	keyDefault             = "default"
	keyConditions          = "conditions"
	keyGotoStep            = "goto_step"
	keyIf                  = "if"
	keyParallel            = "parallel"
	keyRendezvous          = "rendezvous"
	keyResolution          = "resolution"
	keyErrorStrategy       = "error_strategy"
	keyAction              = "action"
	keyMaxRetries          = "max_retries"
	keyRetryDelay          = "retry_delay"
	keyMaxDelay            = "max_delay"
	keyBackoffMultiplier   = "backoff_multiplier"
	keyRetryOn             = "retry_on"
	keyResponseExpectation = "response_expectation"
	keyType                = "type"
	keyItems               = "items"
	keyRequired            = "required"
	keyProperties          = "properties"
	keyMinItems            = "min_items"
	keyMaxItems            = "max_items"
	keyForEach             = "for_each"
	keyAs                  = "as"
	keyMaxParallel         = "max_parallel"
	keyThrottle            = "throttle"
	keyDelay               = "delay"
	keyBatchSize           = "batch_size"
	keyBatchDelay          = "batch_delay"
	keyLoop                = "loop"
	keyWhile               = "while"
	keyMaxIterations       = "max_iterations"
	keyID                  = "id"
	keyCredential          = "credential"
	keyCredentials         = "credentials"
	keySource              = "source"
	keyInjectAs            = "inject_as"
	keyOutputSchema        = "output_schema"
	keyInputSchema         = "input_schema"
	keyVersion             = "version"
	keyFields              = "fields"
	keyEnum                = "enum"
	keyMin                 = "min"
	keyMax                 = "max"
	keyMinLength           = "min_length"
	keyMaxLength           = "max_length"
	keyPattern             = "pattern"
	keyAccept              = "accept"
	keyMaxSize             = "max_size"
	keyVisibleWhen         = "visible_when"
	keyField               = "field"
	keyEquals              = "equals"
	keyIn                  = "in"
	keyQualityGate         = "quality_gate"
	keyRubric              = "rubric"
	keyThreshold           = "threshold"
	keyOnFail              = "on_fail"
	keyExpressionFunctions = "expression_functions"
	keyPackage             = "package"
	keyFunction            = "function"
	keyOrchestrator        = "orchestrator"
	keyExons               = "exons"
	keyAgentic             = "agentic"
	keyMode                = "mode"
	keyTriggers            = "triggers"
	keyTools               = "tools"
	keyInterval            = "interval"
	keyCampaign            = "campaign"
	keyOnChildrenComplete  = "on_children_complete"
	keyPreProcessing       = "pre_processing"
	keyPostProcessing      = "post_processing"
	keyDescription         = "description"
)

// Enum member values the rules branch on. The full sets live in the embedded
// spec; these are the individual members a rule names directly.
const (
	typeObject          = "object"
	typeArray           = "array"
	typeEnum            = "enum"
	typeNumber          = "number"
	typeArrayOfStrings  = "array_of_strings"
	typeFile            = "file"
	actionGoto          = "goto"
	nextMarkerNull      = "null"
	nextMarkerOrch      = "orchestrator"
	triggerTimer        = "timer"
	orchestratorModeOwn = "owner"
)

// reservedStepIDChar is reserved in step IDs: the engine composes a loop
// sub-step's ID as "parent.child", so a literal "." in a top-level step ID would
// collide with that namespace.
const reservedStepIDChar = "."

// schemeSeparator splits an executor URI into scheme and path.
const schemeSeparator = "://"

// Throttle ceilings, mirroring the Go engine's FOR_EACH_MAX_THROTTLE_DELAY (5m)
// and FOR_EACH_MAX_BATCH_DELAY (30m).
const (
	maxThrottleDelay   = 5 * 60 * 1e9  // 5m in nanoseconds
	maxBatchDelay      = 30 * 60 * 1e9 // 30m in nanoseconds
	maxThrottleDelayS  = "5m"
	maxBatchDelayS     = "30m"
	maxUnknownFuncLoop = 64 // bound on the unknown-template-function discovery loop
)

// Stable machine-readable finding codes. These ARE the cross-implementation
// parity contract with aigentflow-flow-validator-js: a consumer compares on
// Code, never on Message, and the two implementations must agree on the SET of
// codes a document produces. Never rename one without the same change upstream.
const (
	// Parse / document shape.
	codeEmptyDocument  = "empty_document"
	codeInvalidRoot    = "invalid_flow_root"
	codeYAMLSyntax     = "yaml_syntax_error"
	codeYAMLWarning    = "yaml_warning"
	codeDuplicateKey   = "duplicate_key"
	codeInvalidType    = "invalid_type"
	codeMissingField   = "missing_required_field"
	codeInvalidValue   = "invalid_field_value"
	codeStepNotFound   = "step_not_found"
	codeReservedStepID = "reserved_step_id_char"

	// Executors.
	codeInvalidExecutorURL   = "invalid_executor_url"
	codeUnknownExecScheme    = "unknown_executor_scheme"
	codeUnreachableStep      = "unreachable_step"
	codePotentialInfiniteLop = "potential_infinite_loop"

	// query / properties / response_expectation.
	codeQueryParamTypeMissing = "query_param_type_missing"
	codePropertyTypeMissing   = "property_type_missing"
	codeUnknownDataType       = "unknown_data_type"
	codeArrayItemsMissing     = "array_items_missing"
	codeArrayItemsTypeInvalid = "array_items_type_invalid"
	codeArrayMinItemsInvalid  = "array_min_items_invalid"
	codeArrayMaxItemsInvalid  = "array_max_items_invalid"
	codeInvalidDataType       = "invalid_data_type"
	codeRespExpArrayItems     = "response_expectation_array_items_missing"

	// error_strategy.
	codeInvalidErrStrategy    = "invalid_error_strategy_action"
	codeGotoStepMissing       = "goto_step_missing"
	codeInvalidDuration       = "invalid_duration"
	codeInvalidBackoffMult    = "invalid_backoff_multiplier"
	codeInvalidRetryOnCategry = "invalid_retry_on_category"

	// next.
	codeOrchNextRequiresOrch = "orchestrator_next_requires_orchestrator"

	// expression_functions.
	codeInvalidExprFunction = "invalid_expression_function"

	// for_each / loop / throttle.
	codeThrottleDelayMax      = "throttle_delay_exceeds_max"
	codeThrottleBatchSize     = "throttle_invalid_batch_size"
	codeThrottleBatchNoSize   = "throttle_batch_delay_without_size"
	codeThrottleBatchDelayMax = "throttle_batch_delay_exceeds_max"
	codeForEachItemsRequired  = "for_each_items_required"
	codeForEachMutualExcl     = "for_each_mutual_exclusion"
	codeForEachMaxParallel    = "for_each_invalid_max_parallel"
	codeForEachResolution     = "for_each_invalid_resolution"
	codeLoopWhileRequired     = "loop_while_required"
	codeLoopMaxIterRequired   = "loop_max_iterations_required"
	codeLoopMaxIterRange      = "loop_max_iterations_range"
	codeLoopStepsRequired     = "loop_steps_required"
	codeLoopStepIDRequired    = "loop_step_id_required"
	codeLoopStepIDDuplicate   = "loop_step_id_duplicate"
	codeLoopStepExecRequired  = "loop_step_executor_required"
	codeLoopMutualExclForEach = "loop_mutual_exclusion_for_each"
	codeLoopMutualExclExec    = "loop_mutual_exclusion_executor"

	// credential bindings.
	codeCredMutualExclusive = "cred_bind_mutual_exclusive"
	codeCredInvalidSource   = "cred_bind_invalid_source"
	codeCredSourceFormat    = "cred_bind_source_format"
	codeCredInjectAsEmpty   = "cred_bind_inject_as_empty"
	codeCredShorthandSource = "cred_bind_shorthand_source"
	codeCredShorthandFormat = "cred_bind_shorthand_format"

	// input_schema / output_schema (shared code set — the Go reference reuses
	// ValidateInputSchemaDefinition verbatim for a step's output_schema).
	codeISInvalidVersion     = "input_schema_invalid_version"
	codeISInvalidFieldName   = "input_schema_invalid_field_name"
	codeISDuplicateFieldName = "input_schema_duplicate_field_name"
	codeISUnknownType        = "input_schema_unknown_type"
	codeISEnumEmpty          = "input_schema_enum_empty"
	codeISInvalidRange       = "input_schema_invalid_range"
	codeISConstraintMismatch = "input_schema_constraint_type_mismatch"
	codeISConstraintRange    = "input_schema_constraint_out_of_range"
	codeISVisibleWhenNoPred  = "input_schema_visible_when_no_predicate"
	codeISVisibleWhenUnknown = "input_schema_visible_when_unknown_field"
	codeISPatternTooLong     = "input_schema_pattern_too_long"
	codeISInvalidPattern     = "input_schema_invalid_pattern"
	codeISFileAfterParam     = "input_schema_file_after_parametric"

	// quality_gate.
	codeQGMissingRubric     = "quality_gate_missing_rubric"
	codeQGThresholdRange    = "quality_gate_threshold_out_of_range"
	codeQGOnFailUnsupported = "quality_gate_on_fail_unsupported"
	codeQGInvalidOnFail     = "quality_gate_invalid_on_fail"
	codeQGGotoMissing       = "quality_gate_goto_missing"
	codeQGGotoSelf          = "quality_gate_goto_self"
	codeQGOnComposite       = "quality_gate_on_composite"
	codeQGOnParallelMember  = "quality_gate_on_parallel_member"

	// orchestrator / campaign.
	codeOrchExonsRequired   = "orchestrator_exons_required"
	codeOrchModeInvalid     = "orchestrator_mode_invalid"
	codeOrchOwnerNeedsYield = "orchestrator_owner_needs_yield"
	codeOrchTriggerUnknown  = "orchestrator_trigger_unknown"
	codeOrchTimerNoInterval = "orchestrator_timer_no_interval"
	codeOrchTimerBadInterv  = "orchestrator_timer_bad_interval"
	codeOrchToolUnknown     = "orchestrator_tool_unknown"
	codeOrchNotAgentic      = "orchestrator_not_agentic"
	codeCampaignNeedsOrch   = "campaign_requires_orchestrator"
	codeCampaignHandoffStep = "campaign_handoff_step_unknown"

	// templates.
	codeTemplateSyntax   = "template_syntax_error"
	codeTemplateFuncUnkn = "template_function_unknown"
)
