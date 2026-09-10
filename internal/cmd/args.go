package cmd

import (
	"github.com/urfave/cli/v3"
)

func sourceDestinationArgs(cmd *cli.Command, requireDestination bool) (src, dst string, err error) {
	count := cmd.NArg()
	if count < 1 || count > 2 || (requireDestination && count != 2) {
		if requireDestination {
			return "", "", invalidArgument("需要提供 <src> <dst>")
		}
		return "", "", invalidArgument("需要提供 <src> [dst]")
	}
	return cmd.Args().Get(0), cmd.Args().Get(1), nil
}

func sourceArg(cmd *cli.Command) (string, error) {
	if cmd.NArg() != 1 {
		return "", invalidArgument("需要提供 <src>")
	}
	return cmd.Args().Get(0), nil
}

func requiredArg(cmd *cli.Command, name string) (string, error) {
	if cmd.NArg() != 1 {
		return "", invalidArgument("需要提供 <%s>", name)
	}
	return cmd.Args().Get(0), nil
}

func requiredOptionalArgs(cmd *cli.Command, firstName, secondName string) (first, second string, err error) {
	if cmd.NArg() < 1 || cmd.NArg() > 2 {
		return "", "", invalidArgument("需要提供 <%s> [%s]", firstName, secondName)
	}
	return cmd.Args().Get(0), cmd.Args().Get(1), nil
}

func optionalArg(cmd *cli.Command, name string) (string, error) {
	if cmd.NArg() > 1 {
		return "", invalidArgument("最多提供一个 [%s]", name)
	}
	return cmd.Args().Get(0), nil
}

func s3UploadArgs(cmd *cli.Command) (src, key string, err error) {
	return requiredOptionalArgs(cmd, "src", "key")
}

func s3DownloadArgs(cmd *cli.Command) (key, dst string, err error) {
	return requiredOptionalArgs(cmd, "key", "dst")
}

func s3KeyArg(cmd *cli.Command) (string, error) {
	return requiredArg(cmd, "key")
}

func s3PrefixArg(cmd *cli.Command) (string, error) {
	return optionalArg(cmd, "prefix")
}
