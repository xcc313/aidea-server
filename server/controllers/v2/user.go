package v2

import (
	"context"
	"fmt"
	"github.com/mylxsw/aidea-server/config"
	"github.com/mylxsw/aidea-server/pkg/ai/chat"
	"github.com/mylxsw/aidea-server/pkg/repo"
	"github.com/mylxsw/aidea-server/pkg/service"
	"github.com/mylxsw/aidea-server/pkg/youdao"
	"github.com/mylxsw/aidea-server/server/auth"
	"github.com/mylxsw/aidea-server/server/controllers/common"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"github.com/mylxsw/go-utils/array"
	"github.com/mylxsw/go-utils/must"
	"net/http"
	"strconv"
	"strings"
)

type UserController struct {
	translater youdao.Translater `autowire:"@"`
	repo       *repo.Repository  `autowire:"@"`
	conf       *config.Config    `autowire:"@"`
}

func NewUserController(resolver infra.Resolver) web.Controller {
	ctl := &UserController{}
	resolver.MustAutoWire(ctl)
	return ctl
}

func (ctl *UserController) Register(router web.Router) {
	router.Group("/users", func(router web.Router) {
		// 自定义首页模型
		router.Post("/custom/home-models", ctl.UpdateCustomHomeModels)
	})
}

// UpdateCustomHomeModels 自定义首页模型
// TODO 这里代码有些乱，特别是异常处理部分
func (ctl *UserController) UpdateCustomHomeModels(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	params := array.Filter(strings.Split(webCtx.Input("models"), ","), func(item string, index int) bool {
		return item != ""
	})

	if len(params) == 0 {
		return webCtx.JSON(web.M{})
	}

	if len(params) < 2 {
		return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrInvalidRequest), http.StatusBadRequest)
	}

	models := array.ToMap(chat.Models(ctl.conf, true), func(item chat.Model, _ int) string {
		return item.ID
	})

	homeModels := array.Map(params, func(item string, _ int) repo.HomeModelV2 {
		segs := strings.SplitN(item, "|", 2)
		if len(segs) != 2 {
			panic("invalid home model format")
		}

		res := repo.HomeModelV2{}
		res.Type, res.ID = segs[0], segs[1]

		switch res.Type {
		case service.HomeModelTypeRoomGallery:
			room, err := ctl.repo.Room.GalleryItem(ctx, int64(must.Must(strconv.Atoi(res.ID))))
			if err != nil {
				panic(fmt.Errorf("get room gallery item failed: %v", err))
			}

			res.Name = room.Name
			model, ok := models[room.Vendor+":"+room.Model]
			if ok {
				res.SupportVision = model.SupportVision
			}
		case service.HomeModelTypeRooms:
			room, err := ctl.repo.Room.Room(ctx, user.ID, int64(must.Must(strconv.Atoi(res.ID))))
			if err != nil {
				panic(fmt.Errorf("get room item failed: %v", err))
			}

			res.Name = room.Name
			model, ok := models[room.Vendor+":"+room.Model]
			if ok {
				res.SupportVision = model.SupportVision
			}
		case service.HomeModelTypeModel:
			model, ok := models[res.ID]
			if !ok {
				panic(fmt.Errorf("model not found: %s", res.ID))
			}

			res.Name = model.ShortName
			res.SupportVision = model.SupportVision
		}

		return res
	})

	cus, err := ctl.repo.User.CustomConfig(ctx, user.ID)
	if err != nil {
		log.WithFields(log.Fields{"user_id": user.ID}).Errorf("get user custom config failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrInternalError), http.StatusInternalServerError)
	}

	cus.HomeModelsV2 = homeModels
	if err := ctl.repo.User.UpdateCustomConfig(ctx, user.ID, *cus); err != nil {
		log.WithFields(log.Fields{"user_id": user.ID}).Errorf("update user custom config failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.translater, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{})
}
