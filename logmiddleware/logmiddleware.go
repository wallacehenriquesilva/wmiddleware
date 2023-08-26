package logmiddleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/rs/zerolog"
	"github.com/wallacehenriquesilva/wlog"
)

const (
	httpRequestField      = "http_request"
	correlationIDField    = "correlation_id"
	elapsedTimeField      = "elapsed_time_ms"
	statusCodeField       = "status_code"
	httpSchemaFieldValue  = "http"
	httpsSchemaFieldValue = "https"
	schemaField           = "scheme"
	headerField           = "header"
	requestURLField       = "request_url"
	requestMethodField    = "request_method"
	requestPathField      = "request_path"
	remoteIPField         = "remote_ip"
	protocolField         = "protocol"
	correlationIDHeader   = "X-Correlation-ID"
	authorizationHeader   = "authorization"
	cookiesHeader         = "cookie"
	setCookiesHeader      = "set-cookie"
)

type logResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		correlationID := getCorrelationID(r)

		ctx := context.WithValue(r.Context(), correlationIDField, correlationID)

		r = r.WithContext(ctx)

		lg := wlog.NewDefaultLogger()

		lg.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str(correlationIDField, correlationID)
		})

		w.Header().Add(correlationIDHeader, correlationID)

		lrw := newLoggingResponseWriter(w)

		r = r.WithContext(lg.WithContext(r.Context()))

		defer func() {
			panicVal := recover()
			if panicVal != nil {
				lrw.statusCode = http.StatusInternalServerError
				panic(panicVal)
			}

			logFields := buildRequestLogFields(r)

			elapsedTimeMS := time.Since(startTime) / time.Millisecond

			(logFields[httpRequestField].(map[string]any))[elapsedTimeField] = elapsedTimeMS
			(logFields[httpRequestField].(map[string]any))[statusCodeField] = lrw.statusCode

			lg.
				Info().
				Fields(logFields).
				Msg("Request returned")
		}()

		lg.
			Info().
			Fields(buildRequestLogFields(r)).
			Msg("Request received")

		next.ServeHTTP(lrw, r)
	})
}

func newLoggingResponseWriter(w http.ResponseWriter) *logResponseWriter {
	return &logResponseWriter{w, http.StatusOK}
}

func getCorrelationID(r *http.Request) string {
	correlationID := r.Header.Get(correlationIDHeader)

	if correlationID == "" {
		correlationIDRaw, _ := uuid.NewV4()
		correlationID = correlationIDRaw.String()
	}

	return correlationID
}

func buildRequestLogFields(r *http.Request) map[string]interface{} {
	scheme := httpSchemaFieldValue
	if r.TLS != nil {
		scheme = httpsSchemaFieldValue
	}

	requestURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)

	requestFields := map[string]interface{}{
		requestURLField:    requestURL,
		requestMethodField: r.Method,
		requestPathField:   r.URL.Path,
		remoteIPField:      r.RemoteAddr,
		protocolField:      r.Proto,
	}

	requestFields[schemaField] = scheme

	if len(r.Header) > 0 {
		requestFields[headerField] = buildHeaderLogFields(r.Header)
	}

	return map[string]interface{}{
		httpRequestField: requestFields,
	}
}

func buildHeaderLogFields(requestHeaders http.Header) map[string]string {
	headers := make(map[string]string, len(requestHeaders))

	for k, v := range requestHeaders {
		k = strings.ToLower(k)
		switch {
		case len(v) == 0:
			continue
		case len(v) == 1:
			headers[k] = v[0]
		default:
			headers[k] = fmt.Sprintf("[%s]", strings.Join(v, "], ["))
		}

		if k == authorizationHeader || k == cookiesHeader || k == setCookiesHeader {
			headers[k] = "***"
		}
	}

	return headers
}
