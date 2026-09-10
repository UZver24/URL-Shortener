package model

import "errors"

var (
	// ErrLinkNotFound — ссылка с таким short_code не найдена
	ErrLinkNotFound = errors.New("link not found")

	// ErrLinkAlreadyExists — ссылка с таким original_url уже существует
	ErrLinkAlreadyExists = errors.New("link already exists")

	// ErrShortCodeAlreadyTaken — пользовательский short_code уже занят
	ErrShortCodeAlreadyTaken = errors.New("short code already taken")

	// ErrInvalidInput — некорректные входные данные (пустой URL, невалидный формат)
	ErrInvalidInput = errors.New("invalid input")

	// ErrInternalServer — внутренняя ошибка сервера (проблемы с БД, паника)
	ErrInternalServer = errors.New("internal server error")
)
