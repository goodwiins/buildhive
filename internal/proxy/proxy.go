package proxy

import (
	"context"
	"fmt"
	"io"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func init() {
	// Register a raw-frame codec under "proto" that passes bytes through
	// for *frameMsg and delegates to the standard proto codec otherwise.
	encoding.RegisterCodecV2(&rawCodec{})
}

// rawCodec replaces the default "proto" codec for transparent frame proxying.
// It type-asserts to *frameMsg and copies raw bytes, avoiding proto reflection.
type rawCodec struct{}

func (rawCodec) Name() string { return "proto" }

func (rawCodec) Marshal(v any) (mem.BufferSlice, error) {
	if m, ok := v.(*frameMsg); ok {
		return mem.BufferSlice{mem.SliceBuffer(m.buf)}, nil
	}
	return nil, fmt.Errorf("rawCodec: unsupported type %T", v)
}

func (rawCodec) Unmarshal(data mem.BufferSlice, v any) error {
	if m, ok := v.(*frameMsg); ok {
		m.buf = data.Materialize()
		return nil
	}
	return fmt.Errorf("rawCodec: unsupported type %T", v)
}

// frameMsg holds a raw gRPC wire frame without proto schema knowledge.
type frameMsg struct{ buf []byte }

// Director decides which backend address to forward an RPC to.
type Director func(ctx context.Context) (string, error)

// Proxy is a transparent gRPC reverse proxy.
type Proxy struct {
	director Director
}

// New creates a Proxy with the given Director.
func New(director Director) *Proxy {
	return &Proxy{director: director}
}

// Handler returns a grpc.StreamHandler suitable for grpc.UnknownServiceHandler that transparently proxies all calls.
func (p *Proxy) Handler() grpc.StreamHandler {
	return func(srv any, stream grpc.ServerStream) error {
		fullMethod, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return status.Error(codes.Internal, "could not determine method name")
		}

		addr, err := p.director(stream.Context())
		if err != nil {
			return status.Errorf(codes.Unavailable, "no builder available: %v", err)
		}

		// Connect to backend buildkitd.
		//nolint:staticcheck
		conn, err := grpc.DialContext(stream.Context(), addr,
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(grpc.ForceCodec(&legacyRawCodec{})),
		)
		if err != nil {
			return status.Errorf(codes.Unavailable, "dial builder %s: %v", addr, err)
		}
		defer conn.Close()

		// Forward incoming metadata to backend.
		md, _ := metadata.FromIncomingContext(stream.Context())
		outCtx := metadata.NewOutgoingContext(stream.Context(), md)

		// Open bidirectional stream to backend.
		clientStream, err := grpc.NewClientStream(outCtx, &grpc.StreamDesc{
			ServerStreams: true,
			ClientStreams: true,
		}, conn, fullMethod)
		if err != nil {
			return status.Errorf(codes.Unavailable, "open client stream: %v", err)
		}

		log.Printf("proxy: %s → %s", fullMethod, addr)

		// Pipe both directions concurrently.
		errCh := make(chan error, 2)
		go pipeFrames(stream, clientStream, errCh)
		go pipeFrames(clientStream, stream, errCh)

		if err := <-errCh; err != nil && err != io.EOF {
			return status.Errorf(codes.Internal, "proxy pipe error: %v", err)
		}
		return nil
	}
}

// BuildkitDirector creates a Director that calls getBuilder to resolve the backend address.
func BuildkitDirector(getBuilder func(ctx context.Context) (string, error)) Director {
	return getBuilder
}

// legacyRawCodec implements the deprecated grpc.Codec interface used with
// grpc.ForceCodec for client streams, doing raw byte pass-through.
type legacyRawCodec struct{}

func (legacyRawCodec) Marshal(v any) ([]byte, error) {
	if m, ok := v.(*frameMsg); ok {
		return m.buf, nil
	}
	return nil, fmt.Errorf("legacyRawCodec: unsupported type %T", v)
}

func (legacyRawCodec) Unmarshal(data []byte, v any) error {
	if m, ok := v.(*frameMsg); ok {
		m.buf = append(m.buf[:0], data...)
		return nil
	}
	return fmt.Errorf("legacyRawCodec: unsupported type %T", v)
}

func (legacyRawCodec) Name() string  { return "proto" }
func (legacyRawCodec) String() string { return "proto" }

type sender interface{ SendMsg(m any) error }
type recver interface{ RecvMsg(m any) error }

func pipeFrames(src recver, dst sender, errCh chan<- error) {
	for {
		msg := &frameMsg{}
		if err := src.RecvMsg(msg); err != nil {
			errCh <- err
			return
		}
		if err := dst.SendMsg(msg); err != nil {
			errCh <- err
			return
		}
	}
}
