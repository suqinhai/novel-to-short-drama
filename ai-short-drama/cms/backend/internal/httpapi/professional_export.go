package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/exportkit"
	"short-drama-cms/backend/internal/store"
)

func registerProfessionalExportRoutes(api *gin.RouterGroup, handler *Handler) {
	api.GET("/projects/:projectID/creation-targets", handler.getCreationTargets)
	api.GET("/projects/:projectID/export-options", handler.getProfessionalExportOptions)
	api.GET("/projects/:projectID/professional-exports", handler.listProfessionalExports)
	api.POST("/projects/:projectID/professional-exports", handler.createProfessionalExport)
	api.GET("/projects/:projectID/professional-exports/:exportID", handler.getProfessionalExport)
	api.GET("/projects/:projectID/professional-exports/:exportID/download", handler.downloadProfessionalExport)
}

func (h *Handler) getCreationTargets(c *gin.Context) {
	item, err := h.store.GetCreationTargetContext(c.Request.Context(), c.Param("projectID"))
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}
func (h *Handler) getProfessionalExportOptions(c *gin.Context) {
	item, err := h.store.GetProfessionalExportOptions(c.Request.Context(), c.Param("projectID"), c.Query("episode_id"))
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}
func (h *Handler) listProfessionalExports(c *gin.Context) {
	items, err := h.store.ListProfessionalExports(c.Request.Context(), c.Param("projectID"), c.Query("episode_id"))
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
func (h *Handler) getProfessionalExport(c *gin.Context) {
	item, err := h.store.GetProfessionalExport(c.Request.Context(), c.Param("projectID"), c.Param("exportID"))
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) createProfessionalExport(c *gin.Context) {
	var input store.CreateProfessionalExportInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EXPORT_REQUEST", err.Error())
		return
	}
	job, err := h.store.CreateProfessionalExport(c.Request.Context(), c.Param("projectID"), input)
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	snapshot, err := h.store.BuildProfessionalExportSnapshot(c.Request.Context(), job)
	if err != nil {
		h.store.FailProfessionalExport(c.Request.Context(), job.ProjectID, job.ExportID, err)
		writeProfessionalExportError(c, err)
		return
	}
	packagePath := store.ExportPackagePath(h.config.StorageDirectory, job)
	manifest, packageHash, err := exportkit.BuildPackage(packagePath, job.Formats, snapshot)
	if err != nil {
		h.store.FailProfessionalExport(c.Request.Context(), job.ProjectID, job.ExportID, err)
		writeProfessionalExportError(c, err)
		return
	}
	job, err = h.store.CompleteProfessionalExport(c.Request.Context(), job.ProjectID, job.ExportID, packagePath, packageHash, manifest)
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": job})
}

func (h *Handler) downloadProfessionalExport(c *gin.Context) {
	job, err := h.store.GetProfessionalExport(c.Request.Context(), c.Param("projectID"), c.Param("exportID"))
	if err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	if job.Status != "ready" || job.PackagePath == nil {
		respondError(c, http.StatusConflict, "EXPORT_NOT_READY", "export package is not ready")
		return
	}
	if err = h.store.ValidateProfessionalExportReady(c.Request.Context(), c.Param("projectID"), c.Param("exportID")); err != nil {
		writeProfessionalExportError(c, err)
		return
	}
	storageRoot, err := filepath.Abs(h.config.StorageDirectory)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "EXPORT_PATH_INVALID", err.Error())
		return
	}
	packagePath, err := filepath.Abs(*job.PackagePath)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "EXPORT_PATH_INVALID", err.Error())
		return
	}
	relative, err := filepath.Rel(storageRoot, packagePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		respondError(c, http.StatusForbidden, "EXPORT_PATH_FORBIDDEN", "export package is outside configured storage")
		return
	}
	if _, err = os.Stat(packagePath); err != nil {
		respondError(c, http.StatusNotFound, "EXPORT_FILE_MISSING", "export package file is missing")
		return
	}
	filename := fmt.Sprintf("%s-%s-v%d.zip", job.ProjectID, job.EpisodeID, job.BundleVersion)
	c.FileAttachment(packagePath, filename)
}

func writeProfessionalExportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EXPORT_NOT_FOUND", err.Error())
	case errors.Is(err, store.ErrValidation):
		respondError(c, http.StatusUnprocessableEntity, "EXPORT_VALIDATION_FAILED", err.Error())
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "EXPORT_CONFLICT", err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
	}
}
