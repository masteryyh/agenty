package builtin

import (
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/tools"
)

func (tool *readFileTool) ApprovalPreview(ctx agentloop.CallContext, input []byte) *tools.CallPreview {
	var args readFileArguments
	if decodeArguments(input, &args) != nil {
		return nil
	}
	path, err := resolvePath(args.Path, ctx.Cwd, false)
	if err != nil {
		return nil
	}
	detail := "Read the file contents."
	if args.StartLine != nil {
		detail += fmt.Sprintf("\nStart line: %d", *args.StartLine)
	}
	if args.EndLine != nil {
		detail += fmt.Sprintf("\nEnd line: %d", *args.EndLine)
	}
	return &tools.CallPreview{Title: "Agenty wants to read this file: " + path, Detail: detail}
}

func (tool *listTool) ApprovalPreview(ctx agentloop.CallContext, input []byte) *tools.CallPreview {
	var args listArguments
	if decodeArguments(input, &args) != nil {
		return nil
	}
	path, err := resolvePath(args.Path, ctx.Cwd, true)
	if err != nil {
		return nil
	}
	return &tools.CallPreview{Title: "Agenty wants to list this directory: " + path, Detail: "List its immediate files and directories."}
}

func (tool *globTool) ApprovalPreview(ctx agentloop.CallContext, input []byte) *tools.CallPreview {
	var args globArguments
	if decodeArguments(input, &args) != nil {
		return nil
	}
	path, err := resolvePath(args.Path, ctx.Cwd, true)
	if err != nil {
		return nil
	}
	detail := "Search directory: " + path
	if args.MaxResults != nil {
		detail += fmt.Sprintf("\nMaximum results: %d", *args.MaxResults)
	}
	return &tools.CallPreview{Title: "Agenty wants to find files matching: " + args.Pattern, Detail: detail}
}

func (tool *grepTool) ApprovalPreview(ctx agentloop.CallContext, input []byte) *tools.CallPreview {
	var args grepArguments
	if decodeArguments(input, &args) != nil {
		return nil
	}
	path, err := resolvePath(args.Path, ctx.Cwd, true)
	if err != nil {
		return nil
	}
	detail := "Search path: " + path
	if args.Glob != "" {
		detail += "\nFile pattern: " + args.Glob
	}
	if args.CaseSensitive != nil {
		detail += fmt.Sprintf("\nCase sensitive: %t", *args.CaseSensitive)
	}
	if args.MaxResults != nil {
		detail += fmt.Sprintf("\nMaximum results: %d", *args.MaxResults)
	}
	return &tools.CallPreview{Title: "Agenty wants to search for: " + args.Pattern, Detail: detail}
}

func (tool *shellTool) ApprovalPreview(_ agentloop.CallContext, input []byte) *tools.CallPreview {
	var args shellArguments
	if decodeArguments(input, &args) != nil {
		return nil
	}
	detail := strings.Join(args.Commands, "\n\n")
	if args.Stdin != nil {
		detail += "\n\nStandard input:\n" + *args.Stdin
	}
	if args.TimeoutMs != nil {
		detail += fmt.Sprintf("\n\nTimeout: %d ms", *args.TimeoutMs)
	}
	if args.MaxOutputLength != nil {
		detail += fmt.Sprintf("\nMaximum output length: %d", *args.MaxOutputLength)
	}
	return &tools.CallPreview{Title: "Agenty wants to run these commands:", Detail: detail}
}

func (tool *applyPatchTool) ApprovalPreview(_ agentloop.CallContext, input []byte) *tools.CallPreview {
	var args struct {
		Patch     string                            `json:"patch"`
		Operation *conversation.ApplyPatchOperation `json:"operation"`
	}
	if json.Unmarshal(input, &args) != nil {
		return nil
	}
	detail := args.Patch
	if args.Operation != nil {
		detail = string(args.Operation.Type) + ": " + args.Operation.Path
		if args.Operation.MoveTo != "" {
			detail += "\nMove to: " + args.Operation.MoveTo
		}
		if args.Operation.Diff != "" {
			detail += "\n\n" + args.Operation.Diff
		}
	}
	return &tools.CallPreview{Title: "Agenty wants to apply these changes:", Detail: detail}
}
