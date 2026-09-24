package service

// CredentialMutationLocks 在同一 CPA 实例的凭证服务间共享，避免不同操作覆盖同一认证文件。
// 零值可用；启停与优先级服务必须注入同一个实例。
type CredentialMutationLocks struct {
	keyedMutex
}
