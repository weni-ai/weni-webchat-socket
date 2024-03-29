// Code generated by MockGen. DO NOT EDIT.
// Source: ./pkg/history/repo.go

// Package history is a generated GoMock package.
package history

import (
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
)

// MockRepo is a mock of Repo interface.
type MockRepo struct {
	ctrl     *gomock.Controller
	recorder *MockRepoMockRecorder
}

// MockRepoMockRecorder is the mock recorder for MockRepo.
type MockRepoMockRecorder struct {
	mock *MockRepo
}

// NewMockRepo creates a new mock instance.
func NewMockRepo(ctrl *gomock.Controller) *MockRepo {
	mock := &MockRepo{ctrl: ctrl}
	mock.recorder = &MockRepoMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRepo) EXPECT() *MockRepoMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockRepo) Get(contactURN, channelUUID string, before *time.Time, limit, page int) ([]MessagePayload, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", contactURN, channelUUID, before, limit, page)
	ret0, _ := ret[0].([]MessagePayload)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockRepoMockRecorder) Get(contactURN, channelUUID, before, limit, page interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockRepo)(nil).Get), contactURN, channelUUID, before, limit, page)
}

// Save mocks base method.
func (m *MockRepo) Save(msg MessagePayload) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Save", msg)
	ret0, _ := ret[0].(error)
	return ret0
}

// Save indicates an expected call of Save.
func (mr *MockRepoMockRecorder) Save(msg interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Save", reflect.TypeOf((*MockRepo)(nil).Save), msg)
}
