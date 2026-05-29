// Package tools — interceptor pipeline.
//
// This file is the package-doc anchor. Implementation split per Sprint 3.5e
// §1.2 (1 file 1 fungsi, max 500 LOC) into multi-file modules:
//
//   - interceptors_workspace.go  : WorkspaceInterceptor + SafeJoin/stringArg helpers
//   - interceptors_kernel.go     : KernelCapabilityInterceptor + capabilityCheckClient
//   - interceptors_shell.go      : ShellSafetyInterceptor
//   - interceptors_sensitive.go  : SensitiveFileInterceptor + sensitiveBasenames/
//                                  sensitiveSuffixes/constitutionFiles + path/bash
//                                  sensitivity helpers (isSensitivePath,
//                                  isSensitiveBashCommand, walkUpAndResolve,
//                                  hasBoundedMatch, isShellBoundary,
//                                  isConstitutionPath, sensitiveError,
//                                  constitutionError, errSensitive)
//   - interceptors_dynamic.go    : Dynamic Protector Rules (DynamicRule,
//                                  ProtectorState, Load/Save/Get state,
//                                  ensureDynamicFresh, isDynamicSensitive,
//                                  HardcodedBasenames/Suffixes facade)
//   - interceptors_history.go    : FileHistoryInterceptor + CompilerVerifyInterceptor
//
// All types stay in `package tools` so method receivers remain co-located.
package tools
