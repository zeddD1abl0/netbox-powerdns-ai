// Command nbpdns reads DNS data from NetBox, and keeps PowerDNS in step with
// it. Run `nbpdns --help` for its commands.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Main(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
