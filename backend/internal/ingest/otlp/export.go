package otlp

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// hasPartialSuccess reports whether an export response tells the client some records were rejected.
func hasPartialSuccess(response proto.Message) bool {
	reflected := response.ProtoReflect()
	field := reflected.Descriptor().Fields().ByName("partial_success")
	return field != nil && reflected.Has(field)
}

// handleHTTP serves one OTLP/HTTP signal: it authenticates, takes a slot, decodes the request,
// exports it, and writes the response, recording the outcome of every step.
func handleHTTP[Request, Response proto.Message](server *Server, signal string, newRequest func() Request, export func(context.Context, Request) (Response, error)) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		startedAt, outcome := time.Now(), "ok"
		defer func() { server.observeHTTPRequest(signal, outcome, time.Since(startedAt)) }()
		authorized, authOutcome := server.authorizeHTTP(responseWriter, request)
		if authOutcome != "" {
			outcome = authOutcome
			return
		}
		request = authorized
		release, admitOutcome := server.admitHTTP(responseWriter, request)
		if admitOutcome != "" {
			outcome = admitOutcome
			return
		}
		defer release()
		message := newRequest()
		encoding, decodeError := server.decodeHTTP(responseWriter, request, message)
		if decodeError != nil {
			outcome = "rejected"
			return
		}
		response, exportError := export(request.Context(), message)
		if exportError != nil {
			outcome = "unavailable"
		} else if hasPartialSuccess(response) {
			outcome = "partial"
		}
		server.writeHTTP(responseWriter, encoding, response, exportError)
	}
}

// handleGRPC serves one OTLP/gRPC signal the same way: authenticate, take a slot, export, and
// record the outcome.
func handleGRPC[Response proto.Message](ctx context.Context, receiver *Receiver, signal string, export func(context.Context) (Response, error)) (response Response, returnError error) {
	startedAt := time.Now()
	defer func() {
		outcome := "ok"
		if returnError != nil {
			outcome = status.Code(returnError).String()
		} else if hasPartialSuccess(response) {
			outcome = "partial"
		}
		receiver.metrics.ObserveOTLPRequest("grpc", signal, outcome, time.Since(startedAt))
	}()
	ctx, authError := receiver.authorizeGRPC(ctx)
	if authError != nil {
		return response, authError
	}
	release, admitError := receiver.admitGRPC()
	if admitError != nil {
		return response, admitError
	}
	defer release()
	response, exportError := export(ctx)
	if exportError != nil {
		var none Response
		return none, status.Error(codes.Unavailable, exportError.Error())
	}
	return response, nil
}
