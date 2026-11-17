package openapi

import "reflect"

// Route describes one API operation. The router owns the list of routes and
// their metadata; this package only renders them, so a new endpoint documents
// itself by declaring its parameters and body types next to its handler.
type Route struct {
	Path          string
	Method        string
	Scope         string
	Tags          []string
	OperationID   string
	SuccessStatus int
	// RequestBody documents an untyped JSON object. Set Request instead to
	// publish the body's schema.
	RequestBody bool
	Public      bool

	// Summary and Description override the text derived from OperationID.
	Summary     string
	Description string
	// Query names reusable query parameters from components/parameters.
	Query []string
	// Headers names reusable header and cookie parameters.
	Headers []string
	// Request is the JSON request body type.
	Request reflect.Type
	// Response is the success body.
	Response Body
}

// BodyKind selects how a success body is wrapped on the wire.
type BodyKind int

const (
	// BodyUntyped documents a JSON object without a schema.
	BodyUntyped BodyKind = iota
	// BodyObject is the type itself.
	BodyObject
	// BodyPage is {"data": [T], "next_cursor": "..."} with next_cursor present while more items follow.
	BodyPage
	// BodyList is {"data": [T]} without pagination.
	BodyList
	// BodyEmpty has no content.
	BodyEmpty
	// BodyEventStream is a Server-Sent Events stream.
	BodyEventStream
)

// Body is a success response body.
type Body struct {
	Kind BodyKind
	Type reflect.Type
}

// Object documents a JSON body that is T itself.
func Object[T any]() Body { return Body{Kind: BodyObject, Type: reflect.TypeFor[T]()} }

// Page documents a cursor-paginated envelope of T.
func Page[T any]() Body { return Body{Kind: BodyPage, Type: reflect.TypeFor[T]()} }

// List documents an unpaginated {"data": [T]} envelope.
func List[T any]() Body { return Body{Kind: BodyList, Type: reflect.TypeFor[T]()} }

// Empty documents a response without content.
func Empty() Body { return Body{Kind: BodyEmpty} }

// EventStream documents a Server-Sent Events response.
func EventStream() Body { return Body{Kind: BodyEventStream} }

// TypeOf returns the reflect.Type of T for Route.Request.
func TypeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }
