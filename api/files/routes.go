package files

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/files", middleware.CorsMiddleware(middleware.DpopMiddleware(h.Files)))

	mux.HandleFunc("/files/upload", middleware.CorsMiddleware(middleware.DpopMiddleware(h.Upload)))
	mux.HandleFunc("/files/finish", middleware.CorsMiddleware(middleware.DpopMiddleware(h.FinishUpload)))

	mux.HandleFunc("/files/download", middleware.CorsMiddleware(middleware.DpopMiddleware(h.Download)))

	mux.HandleFunc("/files/root", middleware.CorsMiddleware(middleware.DpopMiddleware(h.GetRootFolder)))

	mux.HandleFunc("/files/folder", middleware.CorsMiddleware(middleware.DpopMiddleware(h.CreateFolder)))

	mux.HandleFunc("/files/rename", middleware.CorsMiddleware(middleware.DpopMiddleware(h.RenameFile)))
	mux.HandleFunc("/files/trash", middleware.CorsMiddleware(middleware.DpopMiddleware(h.TrashFile)))
	mux.HandleFunc("/files/restore", middleware.CorsMiddleware(middleware.DpopMiddleware(h.RestoreFile)))
	mux.HandleFunc("/files/trash/list", middleware.CorsMiddleware(middleware.DpopMiddleware(h.ListTrash)))
	mux.HandleFunc("/files/trash/empty", middleware.CorsMiddleware(middleware.DpopMiddleware(h.EmptyTrash)))
	mux.HandleFunc("/files/quota", middleware.CorsMiddleware(middleware.DpopMiddleware(h.GetQuota)))
	mux.HandleFunc("/files/move", middleware.CorsMiddleware(middleware.DpopMiddleware(h.MoveFile)))
}
