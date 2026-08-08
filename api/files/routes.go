package files

import (
	"net/http"
	"quartz/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/files", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.Files)))))

	mux.HandleFunc("/files/upload/init", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.InitUpload)))))
	mux.HandleFunc("/files/upload", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.Upload)))))
	mux.HandleFunc("/files/upload/chunk_finish", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.ReportChunkUploadDone)))))
	// mux.HandleFunc("/files/finish", middleware.CorsMiddleware(middleware.DpopMiddleware(h.FinishUpload)))

	mux.HandleFunc("/files/download", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.Download)))))

	mux.HandleFunc("/files/root", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.GetRootFolder)))))

	mux.HandleFunc("/files/folder", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.CreateFolder)))))

	mux.HandleFunc("/files/rename", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.RenameFile)))))
	mux.HandleFunc("/files/trash", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.TrashFile)))))
	mux.HandleFunc("/files/restore", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.RestoreFile)))))
	mux.HandleFunc("/files/trash/list", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.ListTrash)))))
	mux.HandleFunc("/files/trash/empty", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.EmptyTrash)))))
	mux.HandleFunc("/files/quota", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.GetQuota)))))
	mux.HandleFunc("/files/move", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.MoveFile)))))
	mux.HandleFunc("/files/all", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.GetAllFiles)))))
	mux.HandleFunc("/files/path", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.GetFilePath)))))

	mux.HandleFunc("/files/share", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.ShareFolder)))))

	mux.HandleFunc("/files/shared", middleware.CorsMiddleware(middleware.DpopMiddleware(middleware.AccessTokenMiddleware(middleware.LastActivityTracker(h.db, h.GetSharedFolders)))))
}
