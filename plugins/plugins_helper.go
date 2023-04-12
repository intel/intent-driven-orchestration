package pluginsHelper

import (
	"fmt"
	"net/url"
	"os"
)

func IsValidGenericConf(lookBack, pluginManagerPort, port int,
	pythonInterpreter, script, endpoint, pluginManagerEndpoint, mongo string) error {

	if script != "none" {
		_, err := os.Stat(script)
		if err != nil {
			return fmt.Errorf("invalid %s", err)
		}
	}

	if lookBack <= 0 || lookBack > 60*24*7 {
		return fmt.Errorf("invalid lookback value")
	}

	if !isPortNumValid(port) || !isPortNumValid(pluginManagerPort) {
		return fmt.Errorf("invalid port value")
	}

	_, err := url.ParseRequestURI(mongo)

	if err != nil {
		return fmt.Errorf("invalid %s", err)
	}

	if !isStrValid(pythonInterpreter) {
		return fmt.Errorf("invalid analytical script interpreter")
	}

	if !isStrValid(endpoint) ||
		!isStrValid(pluginManagerEndpoint) {
		return fmt.Errorf("invalid endpoint value")
	}

	return nil
}

func isStrValid(str string) bool {
	if str != "" && len(str) < 101 {
		return true
	}
	return false
}

func isPortNumValid(num int) bool {
	if num > 0 && num < 65536 {
		return true
	}
	return false
}
