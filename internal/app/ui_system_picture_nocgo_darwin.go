//go:build darwin && !cgo

package app

import "errors"

func (*app) startSystemPicture() (systemPictureInPicture, error) {
	return nil, errors.New("此构建不支持系统画中画")
}
