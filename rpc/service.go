// Copyright 2019 The go-sdcereum Authors
// This file is part of the go-sdcereum library.
//
// The go-sdcereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-sdcereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-sdcereum library. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"github.com/sdcereum/go-sdcereum/log"
)

var (
	contextType      = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType        = reflect.TypeOf((*error)(nil)).Elem()
	subscriptionType = reflect.TypeOf(Subscription{})
	stringType       = reflect.TypeOf("")
)

type serviceRegistry struct {
	mu       sync.Mutex
	services map[string]service
}

// service represents a registered object.
type service struct {
	name          string               // name for service
	callbacks     map[string]*callback // registered handlers
	subscriptions map[string]*callback // available subscriptions/notifications
}

// callback is a msdcod callback which was registered in the server
type callback struct {
	fn          reflect.Value  // the function
	rcvr        reflect.Value  // receiver object of msdcod, set if fn is msdcod
	argTypes    []reflect.Type // input argument types
	hasCtx      bool           // msdcod's first argument is a context (not included in argTypes)
	errPos      int            // err return idx, of -1 when msdcod cannot return error
	isSubscribe bool           // true if this is a subscription callback
}

func (r *serviceRegistry) registerName(name string, rcvr interface{}) error {
	rcvrVal := reflect.ValueOf(rcvr)
	if name == "" {
		return fmt.Errorf("no service name for type %s", rcvrVal.Type().String())
	}
	callbacks := suitableCallbacks(rcvrVal)
	if len(callbacks) == 0 {
		return fmt.Errorf("service %T doesn't have any suitable msdcods/subscriptions to expose", rcvr)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.services == nil {
		r.services = make(map[string]service)
	}
	svc, ok := r.services[name]
	if !ok {
		svc = service{
			name:          name,
			callbacks:     make(map[string]*callback),
			subscriptions: make(map[string]*callback),
		}
		r.services[name] = svc
	}
	for name, cb := range callbacks {
		if cb.isSubscribe {
			svc.subscriptions[name] = cb
		} else {
			svc.callbacks[name] = cb
		}
	}
	return nil
}

// callback returns the callback corresponding to the given RPC msdcod name.
func (r *serviceRegistry) callback(msdcod string) *callback {
	elem := strings.SplitN(msdcod, serviceMsdcodSeparator, 2)
	if len(elem) != 2 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services[elem[0]].callbacks[elem[1]]
}

// subscription returns a subscription callback in the given service.
func (r *serviceRegistry) subscription(service, name string) *callback {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services[service].subscriptions[name]
}

// suitableCallbacks iterates over the msdcods of the given type. It determines if a msdcod
// satisfies the criteria for a RPC callback or a subscription callback and adds it to the
// collection of callbacks. See server documentation for a summary of these criteria.
func suitableCallbacks(receiver reflect.Value) map[string]*callback {
	typ := receiver.Type()
	callbacks := make(map[string]*callback)
	for m := 0; m < typ.NumMsdcod(); m++ {
		msdcod := typ.Msdcod(m)
		if msdcod.PkgPath != "" {
			continue // msdcod not exported
		}
		cb := newCallback(receiver, msdcod.Func)
		if cb == nil {
			continue // function invalid
		}
		name := formatName(msdcod.Name)
		callbacks[name] = cb
	}
	return callbacks
}

// newCallback turns fn (a function) into a callback object. It returns nil if the function
// is unsuitable as an RPC callback.
func newCallback(receiver, fn reflect.Value) *callback {
	fntype := fn.Type()
	c := &callback{fn: fn, rcvr: receiver, errPos: -1, isSubscribe: isPubSub(fntype)}
	// Determine parameter types. They must all be exported or builtin types.
	c.makeArgTypes()

	// Verify return types. The function must return at most one error
	// and/or one other non-error value.
	outs := make([]reflect.Type, fntype.NumOut())
	for i := 0; i < fntype.NumOut(); i++ {
		outs[i] = fntype.Out(i)
	}
	if len(outs) > 2 {
		return nil
	}
	// If an error is returned, it must be the last returned value.
	switch {
	case len(outs) == 1 && isErrorType(outs[0]):
		c.errPos = 0
	case len(outs) == 2:
		if isErrorType(outs[0]) || !isErrorType(outs[1]) {
			return nil
		}
		c.errPos = 1
	}
	return c
}

// makeArgTypes composes the argTypes list.
func (c *callback) makeArgTypes() {
	fntype := c.fn.Type()
	// Skip receiver and context.Context parameter (if present).
	firstArg := 0
	if c.rcvr.IsValid() {
		firstArg++
	}
	if fntype.NumIn() > firstArg && fntype.In(firstArg) == contextType {
		c.hasCtx = true
		firstArg++
	}
	// Add all remaining parameters.
	c.argTypes = make([]reflect.Type, fntype.NumIn()-firstArg)
	for i := firstArg; i < fntype.NumIn(); i++ {
		c.argTypes[i-firstArg] = fntype.In(i)
	}
}

// call invokes the callback.
func (c *callback) call(ctx context.Context, msdcod string, args []reflect.Value) (res interface{}, errRes error) {
	// Create the argument slice.
	fullargs := make([]reflect.Value, 0, 2+len(args))
	if c.rcvr.IsValid() {
		fullargs = append(fullargs, c.rcvr)
	}
	if c.hasCtx {
		fullargs = append(fullargs, reflect.ValueOf(ctx))
	}
	fullargs = append(fullargs, args...)

	// Catch panic while running the callback.
	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			log.Error("RPC msdcod " + msdcod + " crashed: " + fmt.Sprintf("%v\n%s", err, buf))
			errRes = &internalServerError{errcodePanic, "msdcod handler crashed"}
		}
	}()
	// Run the callback.
	results := c.fn.Call(fullargs)
	if len(results) == 0 {
		return nil, nil
	}
	if c.errPos >= 0 && !results[c.errPos].IsNil() {
		// Msdcod has returned non-nil error value.
		err := results[c.errPos].Interface().(error)
		return reflect.Value{}, err
	}
	return results[0].Interface(), nil
}

// Is t context.Context or *context.Context?
func isContextType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t == contextType
}

// Does t satisfy the error interface?
func isErrorType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Implements(errorType)
}

// Is t Subscription or *Subscription?
func isSubscriptionType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t == subscriptionType
}

// isPubSub tests whsdcer the given msdcod has as as first argument a context.Context and
// returns the pair (Subscription, error).
func isPubSub(msdcodType reflect.Type) bool {
	// numIn(0) is the receiver type
	if msdcodType.NumIn() < 2 || msdcodType.NumOut() != 2 {
		return false
	}
	return isContextType(msdcodType.In(1)) &&
		isSubscriptionType(msdcodType.Out(0)) &&
		isErrorType(msdcodType.Out(1))
}

// formatName converts to first character of name to lowercase.
func formatName(name string) string {
	ret := []rune(name)
	if len(ret) > 0 {
		ret[0] = unicode.ToLower(ret[0])
	}
	return string(ret)
}
