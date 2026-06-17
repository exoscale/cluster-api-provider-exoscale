package domain

import "errors"

var (
	ElasticIPNotFound     = errors.New("elastic ip not found")
	SecurityGroupNotFound = errors.New("security group not found")
)
