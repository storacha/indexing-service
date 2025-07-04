// Code generated by mockery v2.53.4. DO NOT EDIT.

package extmocks

import (
	context "context"

	model "github.com/ipni/go-libipni/find/model"
	mock "github.com/stretchr/testify/mock"

	multihash "github.com/multiformats/go-multihash"
)

// MockIpniFinder is an autogenerated mock type for the Finder type
type MockIpniFinder struct {
	mock.Mock
}

type MockIpniFinder_Expecter struct {
	mock *mock.Mock
}

func (_m *MockIpniFinder) EXPECT() *MockIpniFinder_Expecter {
	return &MockIpniFinder_Expecter{mock: &_m.Mock}
}

// Find provides a mock function with given fields: _a0, _a1
func (_m *MockIpniFinder) Find(_a0 context.Context, _a1 multihash.Multihash) (*model.FindResponse, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for Find")
	}

	var r0 *model.FindResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, multihash.Multihash) (*model.FindResponse, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, multihash.Multihash) *model.FindResponse); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.FindResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, multihash.Multihash) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockIpniFinder_Find_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Find'
type MockIpniFinder_Find_Call struct {
	*mock.Call
}

// Find is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 multihash.Multihash
func (_e *MockIpniFinder_Expecter) Find(_a0 interface{}, _a1 interface{}) *MockIpniFinder_Find_Call {
	return &MockIpniFinder_Find_Call{Call: _e.mock.On("Find", _a0, _a1)}
}

func (_c *MockIpniFinder_Find_Call) Run(run func(_a0 context.Context, _a1 multihash.Multihash)) *MockIpniFinder_Find_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(multihash.Multihash))
	})
	return _c
}

func (_c *MockIpniFinder_Find_Call) Return(_a0 *model.FindResponse, _a1 error) *MockIpniFinder_Find_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockIpniFinder_Find_Call) RunAndReturn(run func(context.Context, multihash.Multihash) (*model.FindResponse, error)) *MockIpniFinder_Find_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockIpniFinder creates a new instance of MockIpniFinder. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockIpniFinder(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockIpniFinder {
	mock := &MockIpniFinder{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
