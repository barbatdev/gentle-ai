package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/deliveryroute"
)

var deliveryRouteResolve = deliveryroute.Resolve

// RunDeliveryRoute projects the read-only origin-derived delivery route as JSON.
func RunDeliveryRoute(args []string, stdout io.Writer) error {
	cwd, jsonOutput := "", false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--cwd":
			index++
			if index == len(args) || strings.HasPrefix(args[index], "-") {
				return fmt.Errorf("--cwd requires a value; usage: gentle-ai delivery-route --cwd <repo> --json")
			}
			cwd = args[index]
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("unknown delivery-route argument %q; usage: gentle-ai delivery-route --cwd <repo> --json", args[index])
		}
	}
	if cwd == "" || !jsonOutput {
		return fmt.Errorf("usage: gentle-ai delivery-route --cwd <repo> --json")
	}
	result := deliveryRouteResolve(context.Background(), cwd)
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return err
	}
	if result.Status == deliveryroute.StatusBlocked {
		return fmt.Errorf("%w: %s", deliveryroute.ErrBlocked, result.Reason)
	}
	return nil
}
