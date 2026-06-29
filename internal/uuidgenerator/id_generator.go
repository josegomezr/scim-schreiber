package uuidgenerator

import "github.com/google/uuid"

type UUIDGenerator interface {
	NewUUID(id string) string
}

type UUIDGeneratorImpl struct{}

func (u UUIDGeneratorImpl) NewUUID(data string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(data)).String()
}

type UUIDGeneratorMock struct{}

func (u UUIDGeneratorMock) NewUUID(data string) string {
	return "1234"
}
