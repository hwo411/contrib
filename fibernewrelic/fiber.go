package fibernewrelic

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/newrelic/go-agent/v3/newrelic"
)

type Config struct {
	// License parameter is required to initialize newrelic application
	License string
	// AppName parameter passed to set app name, default is fiber-api
	AppName string
	// Enabled parameter passed to enable/disable newrelic
	Enabled bool
	// TransportType can be HTTP or HTTPS, default is HTTP
	// Deprecated: The Transport type now acquiring from request URL scheme internally
	TransportType string
	// Application field is required to use an existing newrelic application
	Application *newrelic.Application
	// ErrorStatusCodeHandler is executed when an error is returned from handler
	// Optional. Default: DefaultErrorStatusCodeHandler
	ErrorStatusCodeHandler func(c *fiber.Ctx, err error) int
}

var ConfigDefault = Config{
	Application:            nil,
	License:                "",
	AppName:                "fiber-api",
	Enabled:                false,
	ErrorStatusCodeHandler: DefaultErrorStatusCodeHandler,
}

func New(cfg Config) fiber.Handler {
	var app *newrelic.Application
	var err error

	if cfg.ErrorStatusCodeHandler == nil {
		cfg.ErrorStatusCodeHandler = ConfigDefault.ErrorStatusCodeHandler
	}

	if cfg.Application != nil {
		app = cfg.Application
	} else {
		if cfg.AppName == "" {
			cfg.AppName = ConfigDefault.AppName
		}

		if cfg.License == "" {
			panic(fmt.Errorf("unable to create New Relic Application -> License can not be empty"))
		}

		app, err = newrelic.NewApplication(
			newrelic.ConfigAppName(cfg.AppName),
			newrelic.ConfigLicense(cfg.License),
			newrelic.ConfigEnabled(cfg.Enabled),
		)

		if err != nil {
			panic(fmt.Errorf("unable to create New Relic Application -> %w", err))
		}
	}

	return func(c *fiber.Ctx) error {
		txn := app.StartTransaction(createTransactionName(c))
		defer txn.End()

		scheme := c.Request().URI().Scheme()

		txn.SetWebRequest(newrelic.WebRequest{
			Host:      c.Hostname(),
			Method:    c.Method(),
			Transport: transport(string(scheme)),
			URL: &url.URL{
				Host:     c.Hostname(),
				Scheme:   string(c.Request().URI().Scheme()),
				Path:     string(c.Request().URI().Path()),
				RawQuery: string(c.Request().URI().QueryString()),
			},
		})

		c.SetUserContext(newrelic.NewContext(c.UserContext(), txn))

		handlerErr := c.Next()
		statusCode := c.Context().Response.StatusCode()

		if handlerErr != nil {
			statusCode = cfg.ErrorStatusCodeHandler(c, handlerErr)
			txn.NoticeError(handlerErr)
		}

		txn.SetWebResponse(nil).WriteHeader(statusCode)

		return handlerErr
	}
}

// FromContext returns the Transaction from the context if present, and nil
// otherwise.
func FromContext(c *fiber.Ctx) *newrelic.Transaction {
	return newrelic.FromContext(c.UserContext())
}

func createTransactionName(c *fiber.Ctx) string {
	return fmt.Sprintf("%s %s", c.Request().Header.Method(), c.Request().URI().Path())
}

func transport(schema string) newrelic.TransportType {
	if strings.HasPrefix(schema, "https") {
		return newrelic.TransportHTTPS
	}

	if strings.HasPrefix(schema, "http") {
		return newrelic.TransportHTTP
	}

	return newrelic.TransportUnknown
}

func DefaultErrorStatusCodeHandler(c *fiber.Ctx, err error) int {
	if fiberErr, ok := err.(*fiber.Error); ok {
		return fiberErr.Code
	}

	return c.Context().Response.StatusCode()
}
