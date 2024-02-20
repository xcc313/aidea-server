package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mylxsw/aidea-server/pkg/repo"
	"github.com/mylxsw/aidea-server/pkg/youdao"
	"net/http"
	"time"

	"github.com/mylxsw/aidea-server/config"
	"github.com/mylxsw/aidea-server/internal/queue"
	"github.com/mylxsw/aidea-server/server/auth"
	"github.com/mylxsw/aidea-server/server/controllers/common"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
)

type TaskController struct {
	conf       *config.Config
	queueRepo  *repo.QueueRepo   `autowire:"@"`
	translater youdao.Translater `autowire:"@"`
}

func NewTaskController(resolver infra.Resolver, conf *config.Config) web.Controller {
	ctl := TaskController{conf: conf}
	resolver.MustAutoWire(&ctl)
	return &ctl
}

func (ctl *TaskController) Register(router web.Router) {
	router.Group("/tasks", func(router web.Router) {
		router.Get("/{task_id}/status", ctl.TaskStatus)
	})
}

// TaskStatus 任务状态查询
func (ctl *TaskController) TaskStatus(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	taskID := webCtx.PathVar("task_id")
	task, err := ctl.queueRepo.Task(ctx, taskID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSONError(common.ErrNotFound, http.StatusNotFound)
		}
		return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrInternalError), http.StatusInternalServerError)
	}

	if task.Uid != user.ID {
		return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrNotFound), http.StatusNotFound)
	}

	if repo.QueueTaskStatus(task.Status) == repo.QueueTaskStatusSuccess {
		var taskResult queue.CompletionResult
		if err := json.Unmarshal([]byte(task.Result), &taskResult); err != nil {
			log.With(task).Errorf("unmarshal task result failed: %v", err)
			return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrInternalError), http.StatusInternalServerError)
		}
		res := web.M{
			"status":       task.Status,
			"origin_image": taskResult.OriginImage,
			"resources":    taskResult.Resources,
			"valid_before": taskResult.ValidBefore.Format(time.RFC3339),
		}

		if taskResult.Width > 0 {
			res["width"] = taskResult.Width
		}

		if taskResult.Height > 0 {
			res["height"] = taskResult.Height
		}

		return webCtx.JSON(res)
	}

	if repo.QueueTaskStatus(task.Status) == repo.QueueTaskStatusFailed {
		var errResult queue.ErrorResult
		if err := json.Unmarshal([]byte(task.Result), &errResult); err != nil {
			log.With(task).Errorf("unmarshal task result failed: %v", err)
			return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrInternalError), http.StatusInternalServerError)
		}

		return webCtx.JSON(web.M{
			"status": task.Status,
			"errors": errResult.Errors,
		})
	}

	return webCtx.JSON(web.M{
		"status": task.Status,
	})
}
