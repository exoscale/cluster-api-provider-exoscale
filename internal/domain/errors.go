package domain

import "errors"

var (
	ErrElasticIPNotFound          = errors.New("elastic ip not found")
	ErrSecurityGroupNotFound      = errors.New("security group not found")
	ErrSecurityGroupAlreadyExists = errors.New("security group already exists")
	ErrInstanceNotFound           = errors.New("instance not found")
)
