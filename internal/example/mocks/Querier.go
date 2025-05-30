// Code generated by mockery v2.53.3. DO NOT EDIT.

package mocks

import (
	example "NYCU-SDC/core-system-backend/internal/example"
	context "context"

	mock "github.com/stretchr/testify/mock"

	uuid "github.com/google/uuid"
)

// Querier is an autogenerated mock type for the Querier type
type Querier struct {
	mock.Mock
}

// Create provides a mock function with given fields: ctx, name
func (_m *Querier) Create(ctx context.Context, name string) (example.Scoreboard, error) {
	ret := _m.Called(ctx, name)

	if len(ret) == 0 {
		panic("no return value specified for Create")
	}

	var r0 example.Scoreboard
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (example.Scoreboard, error)); ok {
		return rf(ctx, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) example.Scoreboard); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Get(0).(example.Scoreboard)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, id
func (_m *Querier) Delete(ctx context.Context, id uuid.UUID) error {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) error); ok {
		r0 = rf(ctx, id)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetAll provides a mock function with given fields: ctx
func (_m *Querier) GetAll(ctx context.Context) ([]example.Scoreboard, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetAll")
	}

	var r0 []example.Scoreboard
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) ([]example.Scoreboard, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) []example.Scoreboard); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]example.Scoreboard)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetByID provides a mock function with given fields: ctx, id
func (_m *Querier) GetByID(ctx context.Context, id uuid.UUID) (example.Scoreboard, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for GetByID")
	}

	var r0 example.Scoreboard
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) (example.Scoreboard, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) example.Scoreboard); ok {
		r0 = rf(ctx, id)
	} else {
		r0 = ret.Get(0).(example.Scoreboard)
	}

	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Update provides a mock function with given fields: ctx, arg
func (_m *Querier) Update(ctx context.Context, arg example.UpdateParams) (example.Scoreboard, error) {
	ret := _m.Called(ctx, arg)

	if len(ret) == 0 {
		panic("no return value specified for Update")
	}

	var r0 example.Scoreboard
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, example.UpdateParams) (example.Scoreboard, error)); ok {
		return rf(ctx, arg)
	}
	if rf, ok := ret.Get(0).(func(context.Context, example.UpdateParams) example.Scoreboard); ok {
		r0 = rf(ctx, arg)
	} else {
		r0 = ret.Get(0).(example.Scoreboard)
	}

	if rf, ok := ret.Get(1).(func(context.Context, example.UpdateParams) error); ok {
		r1 = rf(ctx, arg)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewQuerier creates a new instance of Querier. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewQuerier(t interface {
	mock.TestingT
	Cleanup(func())
}) *Querier {
	mock := &Querier{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
