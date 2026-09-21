package files

import (
	"net/http"
	"quartz/config"
	"quartz/internal/middleware"
	"quartz/pkg/httputils"
	"time"

	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, rdb *redis.Client) {
	limitIP := middleware.RateLimitIP(rdb, "files", config.Cfg.RateLimits.FilesIPRate, time.Second, config.Cfg.RateLimits.FilesIPBurst)
	limitUser := middleware.RateLimitUser(rdb, "files", config.Cfg.RateLimits.FilesUserRate, time.Second, config.Cfg.RateLimits.FilesUserBurst)

	wrap := func(handler httputils.APIHandler) http.HandlerFunc {
		return httputils.Wrap(
			middleware.CorsMiddleware(
				limitIP(
					middleware.DpopMiddleware(
						middleware.AccessTokenMiddleware(
							limitUser(
								middleware.LastActivityTracker(h.db, handler),
							),
						),
					),
				),
			),
		)
	}

	mux.HandleFunc("/files", wrap(h.Files))

	mux.HandleFunc("/files/upload/init", wrap(h.InitUpload))
	mux.HandleFunc("/files/upload", wrap(h.Upload))
	mux.HandleFunc("/files/upload/chunk_finish", wrap(h.ReportChunkUploadDone))

	mux.HandleFunc("/files/download", wrap(h.DownloadUrls))
	mux.HandleFunc("/files/root", wrap(h.GetRootFolder))
	mux.HandleFunc("/files/folder", wrap(h.CreateFolder))
	mux.HandleFunc("/files/rename", wrap(h.RenameFile))
	mux.HandleFunc("/files/trash", wrap(h.TrashFile))
	mux.HandleFunc("/files/restore", wrap(h.RestoreFile))
	mux.HandleFunc("/files/trash/list", wrap(h.ListTrash))
	mux.HandleFunc("/files/trash/empty", wrap(h.EmptyTrash))
	mux.HandleFunc("/files/quota", wrap(h.GetQuota))
	mux.HandleFunc("/files/move", wrap(h.MoveFile))
	mux.HandleFunc("/files/all", wrap(h.GetAllFiles))
	mux.HandleFunc("/files/path", wrap(h.GetFilePath))
	mux.HandleFunc("/files/share", wrap(h.ShareFolder))
	mux.HandleFunc("/files/shared", wrap(h.GetSharedFolders))
}
