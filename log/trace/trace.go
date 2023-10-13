package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"maps"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/pool"
)

// https://www.w3.org/TR/trace-context/

var logtrace bool
var sloger *slog.Logger

func init() {
	if str := os.Getenv("LOG_TRACE"); str != "" && str != "<LOG_TRACE>" && str != "0" && str != "1" {
		panic("[log] os env LOG_TRACE error,must in [0,1]")
	} else {
		logtrace = str == "1"
	}
}
func SetSloger(l *slog.Logger) {
	sloger = l
}

type TraceID [16]byte

func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}
func (t TraceID) IsEmpty() bool {
	return t.String() == strings.Repeat("0", 32)
}
func TraceIDFromHex(str string) (TraceID, error) {
	if len(str) != 32 {
		return TraceID{}, cerror.ErrDataBroken
	}
	tmptid, e := hex.DecodeString(str)
	if e != nil {
		return TraceID{}, cerror.ErrDataBroken
	}
	tid := TraceID{}
	for i, v := range tmptid {
		tid[i] = v
	}
	return tid, nil
}

type SpanID [8]byte

func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}
func (s SpanID) IsEmpty() bool {
	return s.String() == strings.Repeat("0", 16)
}
func SpanIDFromHex(str string) (SpanID, error) {
	if len(str) != 16 {
		return SpanID{}, cerror.ErrDataBroken
	}
	tmpsid, e := hex.DecodeString(str)
	if e != nil {
		return SpanID{}, cerror.ErrDataBroken
	}
	sid := SpanID{}
	for i, v := range tmpsid {
		sid[i] = v
	}
	return sid, nil
}

type TraceState map[string]string

type SpanData struct {
	tid   TraceID
	sid   SpanID
	state TraceState
}

func NewSpanData(tid TraceID, sid SpanID) *SpanData {
	return &SpanData{tid: tid, sid: sid, state: make(map[string]string)}
}

func (sd *SpanData) child() *SpanData {
	newsid := SpanID{}
	for {
		rand.Read(newsid[:])
		if newsid.IsEmpty() {
			continue
		}
		break
	}
	if sd.IsEmpty() {
		newtid := TraceID{}
		for {
			rand.Read(newtid[:])
			if newtid.IsEmpty() {
				continue
			}
			break
		}
		return &SpanData{
			tid:   newtid,
			sid:   newsid,
			state: make(map[string]string),
		}
	}
	return &SpanData{
		tid:   sd.tid,
		sid:   newsid,
		state: make(map[string]string),
	}
}
func (sd *SpanData) IsEmpty() bool {
	return sd.tid.IsEmpty() || sd.sid.IsEmpty()
}
func (sd *SpanData) GetTid() TraceID {
	r := TraceID{}
	for i, v := range sd.tid {
		r[i] = v
	}
	return r
}
func (sd *SpanData) GetSid() SpanID {
	r := SpanID{}
	for i, v := range sd.sid {
		r[i] = v
	}
	return r
}
func (sd *SpanData) GetState() map[string]string {
	r := make(map[string]string)
	maps.Copy(r, sd.state)
	return r
}
func (sd *SpanData) SetStateKV(key, value string) {
	sd.state[key] = value
}
func (sd *SpanData) GetStateKV(key string) (string, bool) {
	v, ok := sd.state[key]
	return v, ok
}
func (sd *SpanData) DelStateKV(key string) {
	delete(sd.state, key)
}

func (sd *SpanData) FormatTraceParent() string {
	buf := pool.GetPool().Get(55)
	defer pool.GetPool().Put(&buf)
	buf = buf[:0]
	buf = append(buf, "00-"...)
	buf = append(buf, sd.tid.String()...)
	buf = append(buf, '-')
	buf = append(buf, sd.sid.String()...)
	buf = append(buf, "-01"...)
	return string(buf)
}
func (sd *SpanData) FormatTraceState() string {
	if len(sd.state) == 0 {
		return ""
	}
	length := len(sd.state) - 1 //this is for ','
	for k, v := range sd.state {
		length += len(k) + len(v) + 1 //+1 for '='
	}
	buf := pool.GetPool().Get(length)
	defer pool.GetPool().Put(&buf)
	buf = buf[:0]
	first := true
	for k, v := range sd.state {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, k...)
		buf = append(buf, '=')
		buf = append(buf, v...)
	}
	return string(buf)
}
func ParseTraceParent(traceparentstr string) (tid TraceID, sid SpanID, e error) {
	if traceparentstr == "" {
		return
	}
	pieces := strings.Split(traceparentstr, "-")
	if len(pieces) != 4 && len(pieces[1]) != 32 && len(pieces[2]) != 16 {
		e = cerror.ErrDataBroken
		return
	}
	if tid, e = TraceIDFromHex(pieces[1]); e != nil {
		return
	}
	if sid, e = SpanIDFromHex(pieces[2]); e != nil {
		return
	}
	return
}
func ParseTraceState(tracestatestr string) (map[string]string, error) {
	if tracestatestr == "" {
		return make(map[string]string), nil
	}
	pieces := strings.Split(tracestatestr, ",")
	if len(pieces) > 32 {
		return nil, cerror.ErrDataBroken
	}
	state := make(map[string]string, len(pieces))
	for _, piece := range pieces {
		if piece == "" {
			continue
		}
		kv := strings.Split(piece, "=")
		if len(kv) != 2 {
			return nil, cerror.ErrDataBroken
		}
		state[kv[0]] = kv[1]
	}
	return state, nil
}

type SpanKind int

const (
	Unspecified SpanKind = iota
	Internal
	Server
	Client
	Producer
	Consumer
)

func (sk SpanKind) String() string {
	switch sk {
	case Internal:
		return "internal"
	case Server:
		return "server"
	case Client:
		return "client"
	case Producer:
		return "producer"
	case Consumer:
		return "consumer"
	default:
		return "unspecified"
	}
}

type Span struct {
	name  string
	start int64 //timestamp unixnano
	end   int64 //timestamp unixnano
	kind  SpanKind
	p     *SpanData //parent
	s     *SpanData //self
}

type spankey struct{}

// if forceparent is not nil,new span's parent will be it
// else the span in the context will be the parent
// if the span in the context is nil,the empty parent will be used
func NewSpan(ctx context.Context, name string, kind SpanKind, forceparent *SpanData) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	var s *Span
	if forceparent != nil {
		s = &Span{
			name:  name,
			start: time.Now().UnixNano(),
			kind:  kind,
			p:     forceparent,
			s:     forceparent.child(),
		}
	} else if pspan := SpanFromContext(ctx); pspan == nil {
		emptyp := NewSpanData(TraceID{}, SpanID{})
		s = &Span{
			name:  name,
			start: time.Now().UnixNano(),
			kind:  kind,
			p:     emptyp,
			s:     emptyp.child(),
		}
	} else {
		s = &Span{
			name:  name,
			start: time.Now().UnixNano(),
			kind:  kind,
			p:     pspan.s,
			s:     pspan.s.child(),
		}
	}
	ctx = context.WithValue(ctx, spankey{}, s)
	return ctx, s
}
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	s, ok := ctx.Value(spankey{}).(*Span)
	if !ok {
		return nil
	}
	return s
}
func CloneSpan(ctx context.Context) context.Context {
	s := SpanFromContext(ctx)
	if s != nil {
		return context.WithValue(context.Background(), spankey{}, s)
	}
	return context.Background()
}
func (s *Span) GetSelfSpanData() *SpanData {
	return s.s
}
func (s *Span) GetParentSpanData() *SpanData {
	return s.p
}
func (s *Span) GetStart() int64 {
	return s.start
}
func (s *Span) GetEnd() int64 {
	return s.end
}
func (s *Span) Finish(e error) {
	end := time.Now().UnixNano()
	if !atomic.CompareAndSwapInt64(&s.end, 0, end) {
		return
	}
	if !logtrace {
		return
	}
	attrs := make([]any, 0, 10)
	attrs = append(attrs, slog.String("name", s.name))
	attrs = append(attrs, slog.String("tid", s.s.tid.String()))
	attrs = append(attrs, slog.String("s_sid", s.s.sid.String()))
	attrs = append(attrs, slog.Any("s_state", s.s.state))
	if s.p == nil {
		attrs = append(attrs, slog.String("p_sid", strings.Repeat("0", 16)))
		attrs = append(attrs, slog.String("p_state", "{}"))
	} else {
		attrs = append(attrs, slog.String("p_sid", s.p.sid.String()))
		attrs = append(attrs, slog.Any("p_state", s.p.state))
	}
	attrs = append(attrs, slog.String("kind", s.kind.String()))
	attrs = append(attrs, slog.Int64("duration", s.end-s.start))
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		attrs = append(attrs, slog.Any("error", nil))
	} else {
		attrs = append(attrs, slog.Group("error", slog.Int64("code", int64(ee.Code)), slog.String("msg", ee.Msg)))
	}

	var pcs [1]uintptr
	//skip runtime.Callers ,this function
	runtime.Callers(2, pcs[:])
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "trace", pcs[0])
	r.Add(attrs...)
	sloger.Handler().Handle(context.Background(), r)
}
