// Code generated by mockery v2.50.0. DO NOT EDIT.

package types

import (
	context "context"

	model "github.com/ipni/go-libipni/find/model"
	mock "github.com/stretchr/testify/mock"

	multihash "github.com/multiformats/go-multihash"
)

// MockProviderStore is an autogenerated mock type for the ProviderStore type
type MockProviderStore struct {
	mock.Mock
}

type MockProviderStore_Expecter struct {
	mock *mock.Mock
}

func (_m *MockProviderStore) EXPECT() *MockProviderStore_Expecter {
	return &MockProviderStore_Expecter{mock: &_m.Mock}
}

// Get provides a mock function with given fields: ctx, key
func (_m *MockProviderStore) Get(ctx context.Context, key multihash.Multihash) ([]model.ProviderResult, error) {
	ret := _m.Called(ctx, key)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 []model.ProviderResult
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, multihash.Multihash) ([]model.ProviderResult, error)); ok {
		return rf(ctx, key)
	}
	if rf, ok := ret.Get(0).(func(context.Context, multihash.Multihash) []model.ProviderResult); ok {
		r0 = rf(ctx, key)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]model.ProviderResult)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, multihash.Multihash) error); ok {
		r1 = rf(ctx, key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockProviderStore_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type MockProviderStore_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - ctx context.Context
//   - key multihash.Multihash
func (_e *MockProviderStore_Expecter) Get(ctx interface{}, key interface{}) *MockProviderStore_Get_Call {
	return &MockProviderStore_Get_Call{Call: _e.mock.On("Get", ctx, key)}
}

func (_c *MockProviderStore_Get_Call) Run(run func(ctx context.Context, key multihash.Multihash)) *MockProviderStore_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(multihash.Multihash))
	})
	return _c
}

func (_c *MockProviderStore_Get_Call) Return(_a0 []model.ProviderResult, _a1 error) *MockProviderStore_Get_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockProviderStore_Get_Call) RunAndReturn(run func(context.Context, multihash.Multihash) ([]model.ProviderResult, error)) *MockProviderStore_Get_Call {
	_c.Call.Return(run)
	return _c
}

// Set provides a mock function with given fields: ctx, key, value, expires
func (_m *MockProviderStore) Set(ctx context.Context, key multihash.Multihash, value []model.ProviderResult, expires bool) error {
	ret := _m.Called(ctx, key, value, expires)

	if len(ret) == 0 {
		panic("no return value specified for Set")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, multihash.Multihash, []model.ProviderResult, bool) error); ok {
		r0 = rf(ctx, key, value, expires)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockProviderStore_Set_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Set'
type MockProviderStore_Set_Call struct {
	*mock.Call
}

// Set is a helper method to define mock.On call
//   - ctx context.Context
//   - key multihash.Multihash
//   - value []model.ProviderResult
//   - expires bool
func (_e *MockProviderStore_Expecter) Set(ctx interface{}, key interface{}, value interface{}, expires interface{}) *MockProviderStore_Set_Call {
	return &MockProviderStore_Set_Call{Call: _e.mock.On("Set", ctx, key, value, expires)}
}

func (_c *MockProviderStore_Set_Call) Run(run func(ctx context.Context, key multihash.Multihash, value []model.ProviderResult, expires bool)) *MockProviderStore_Set_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(multihash.Multihash), args[2].([]model.ProviderResult), args[3].(bool))
	})
	return _c
}

func (_c *MockProviderStore_Set_Call) Return(_a0 error) *MockProviderStore_Set_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockProviderStore_Set_Call) RunAndReturn(run func(context.Context, multihash.Multihash, []model.ProviderResult, bool) error) *MockProviderStore_Set_Call {
	_c.Call.Return(run)
	return _c
}

// SetExpirable provides a mock function with given fields: ctx, key, expires
func (_m *MockProviderStore) SetExpirable(ctx context.Context, key multihash.Multihash, expires bool) error {
	ret := _m.Called(ctx, key, expires)

	if len(ret) == 0 {
		panic("no return value specified for SetExpirable")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, multihash.Multihash, bool) error); ok {
		r0 = rf(ctx, key, expires)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockProviderStore_SetExpirable_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SetExpirable'
type MockProviderStore_SetExpirable_Call struct {
	*mock.Call
}

// SetExpirable is a helper method to define mock.On call
//   - ctx context.Context
//   - key multihash.Multihash
//   - expires bool
func (_e *MockProviderStore_Expecter) SetExpirable(ctx interface{}, key interface{}, expires interface{}) *MockProviderStore_SetExpirable_Call {
	return &MockProviderStore_SetExpirable_Call{Call: _e.mock.On("SetExpirable", ctx, key, expires)}
}

func (_c *MockProviderStore_SetExpirable_Call) Run(run func(ctx context.Context, key multihash.Multihash, expires bool)) *MockProviderStore_SetExpirable_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(multihash.Multihash), args[2].(bool))
	})
	return _c
}

func (_c *MockProviderStore_SetExpirable_Call) Return(_a0 error) *MockProviderStore_SetExpirable_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockProviderStore_SetExpirable_Call) RunAndReturn(run func(context.Context, multihash.Multihash, bool) error) *MockProviderStore_SetExpirable_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockProviderStore creates a new instance of MockProviderStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockProviderStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockProviderStore {
	mock := &MockProviderStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
