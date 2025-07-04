// Code generated by mockery v2.53.4. DO NOT EDIT.

package types

import (
	context "context"

	delegation "github.com/storacha/go-ucanto/core/delegation"
	ipld "github.com/storacha/go-ucanto/core/ipld"

	mock "github.com/stretchr/testify/mock"

	peer "github.com/libp2p/go-libp2p/core/peer"
)

// MockService is an autogenerated mock type for the Service type
type MockService struct {
	mock.Mock
}

type MockService_Expecter struct {
	mock *mock.Mock
}

func (_m *MockService) EXPECT() *MockService_Expecter {
	return &MockService_Expecter{mock: &_m.Mock}
}

// Cache provides a mock function with given fields: ctx, provider, claim
func (_m *MockService) Cache(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error {
	ret := _m.Called(ctx, provider, claim)

	if len(ret) == 0 {
		panic("no return value specified for Cache")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, peer.AddrInfo, delegation.Delegation) error); ok {
		r0 = rf(ctx, provider, claim)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockService_Cache_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Cache'
type MockService_Cache_Call struct {
	*mock.Call
}

// Cache is a helper method to define mock.On call
//   - ctx context.Context
//   - provider peer.AddrInfo
//   - claim delegation.Delegation
func (_e *MockService_Expecter) Cache(ctx interface{}, provider interface{}, claim interface{}) *MockService_Cache_Call {
	return &MockService_Cache_Call{Call: _e.mock.On("Cache", ctx, provider, claim)}
}

func (_c *MockService_Cache_Call) Run(run func(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation)) *MockService_Cache_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(peer.AddrInfo), args[2].(delegation.Delegation))
	})
	return _c
}

func (_c *MockService_Cache_Call) Return(_a0 error) *MockService_Cache_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockService_Cache_Call) RunAndReturn(run func(context.Context, peer.AddrInfo, delegation.Delegation) error) *MockService_Cache_Call {
	_c.Call.Return(run)
	return _c
}

// Get provides a mock function with given fields: ctx, claim
func (_m *MockService) Get(ctx context.Context, claim ipld.Link) (delegation.Delegation, error) {
	ret := _m.Called(ctx, claim)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 delegation.Delegation
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, ipld.Link) (delegation.Delegation, error)); ok {
		return rf(ctx, claim)
	}
	if rf, ok := ret.Get(0).(func(context.Context, ipld.Link) delegation.Delegation); ok {
		r0 = rf(ctx, claim)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(delegation.Delegation)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, ipld.Link) error); ok {
		r1 = rf(ctx, claim)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type MockService_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - ctx context.Context
//   - claim ipld.Link
func (_e *MockService_Expecter) Get(ctx interface{}, claim interface{}) *MockService_Get_Call {
	return &MockService_Get_Call{Call: _e.mock.On("Get", ctx, claim)}
}

func (_c *MockService_Get_Call) Run(run func(ctx context.Context, claim ipld.Link)) *MockService_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(ipld.Link))
	})
	return _c
}

func (_c *MockService_Get_Call) Return(_a0 delegation.Delegation, _a1 error) *MockService_Get_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_Get_Call) RunAndReturn(run func(context.Context, ipld.Link) (delegation.Delegation, error)) *MockService_Get_Call {
	_c.Call.Return(run)
	return _c
}

// Publish provides a mock function with given fields: ctx, claim
func (_m *MockService) Publish(ctx context.Context, claim delegation.Delegation) error {
	ret := _m.Called(ctx, claim)

	if len(ret) == 0 {
		panic("no return value specified for Publish")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, delegation.Delegation) error); ok {
		r0 = rf(ctx, claim)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockService_Publish_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Publish'
type MockService_Publish_Call struct {
	*mock.Call
}

// Publish is a helper method to define mock.On call
//   - ctx context.Context
//   - claim delegation.Delegation
func (_e *MockService_Expecter) Publish(ctx interface{}, claim interface{}) *MockService_Publish_Call {
	return &MockService_Publish_Call{Call: _e.mock.On("Publish", ctx, claim)}
}

func (_c *MockService_Publish_Call) Run(run func(ctx context.Context, claim delegation.Delegation)) *MockService_Publish_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(delegation.Delegation))
	})
	return _c
}

func (_c *MockService_Publish_Call) Return(_a0 error) *MockService_Publish_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockService_Publish_Call) RunAndReturn(run func(context.Context, delegation.Delegation) error) *MockService_Publish_Call {
	_c.Call.Return(run)
	return _c
}

// Query provides a mock function with given fields: ctx, q
func (_m *MockService) Query(ctx context.Context, q Query) (QueryResult, error) {
	ret := _m.Called(ctx, q)

	if len(ret) == 0 {
		panic("no return value specified for Query")
	}

	var r0 QueryResult
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, Query) (QueryResult, error)); ok {
		return rf(ctx, q)
	}
	if rf, ok := ret.Get(0).(func(context.Context, Query) QueryResult); ok {
		r0 = rf(ctx, q)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(QueryResult)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, Query) error); ok {
		r1 = rf(ctx, q)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_Query_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Query'
type MockService_Query_Call struct {
	*mock.Call
}

// Query is a helper method to define mock.On call
//   - ctx context.Context
//   - q Query
func (_e *MockService_Expecter) Query(ctx interface{}, q interface{}) *MockService_Query_Call {
	return &MockService_Query_Call{Call: _e.mock.On("Query", ctx, q)}
}

func (_c *MockService_Query_Call) Run(run func(ctx context.Context, q Query)) *MockService_Query_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(Query))
	})
	return _c
}

func (_c *MockService_Query_Call) Return(_a0 QueryResult, _a1 error) *MockService_Query_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_Query_Call) RunAndReturn(run func(context.Context, Query) (QueryResult, error)) *MockService_Query_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockService creates a new instance of MockService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockService(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockService {
	mock := &MockService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
