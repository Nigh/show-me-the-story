package main

import "fmt"

// ponytail: per-tool-call scratch fields on AgentContext; single-threaded agent loop only.
func (ctx *AgentContext) clearToolMsg() {
	ctx.toolMsgKey = ""
	ctx.toolMsgArgs = nil
}

func (ctx *AgentContext) setToolMsg(key string, args ...any) {
	ctx.toolMsgKey = key
	ctx.toolMsgArgs = msgArgsToStrings(args...)
}

func (ctx *AgentContext) takeToolMsg() (string, []string) {
	k, a := ctx.toolMsgKey, ctx.toolMsgArgs
	ctx.clearToolMsg()
	return k, a
}

func projectLang(ctx *AgentContext) string {
	if ctx == nil || ctx.Config == nil {
		return LangZH
	}
	return NormalizeLanguage(ctx.Config.Language)
}

func agentMsg(ctx *AgentContext, key string, args ...any) string {
	ctx.setToolMsg(key, args...)
	return T(projectLang(ctx), key, args...)
}

func agentErr(ctx *AgentContext, key string, args ...any) error {
	return fmt.Errorf("%s", T(projectLang(ctx), key, args...))
}
