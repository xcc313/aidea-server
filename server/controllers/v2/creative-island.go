package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mylxsw/aidea-server/pkg/ai/dashscope"
	"github.com/mylxsw/aidea-server/pkg/ai/xfyun"
	"github.com/mylxsw/aidea-server/pkg/misc"
	"github.com/mylxsw/aidea-server/pkg/repo"
	"github.com/mylxsw/aidea-server/pkg/service"
	"github.com/mylxsw/aidea-server/pkg/uploader"
	"github.com/mylxsw/aidea-server/pkg/youdao"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fvbommel/sortorder"
	"github.com/mylxsw/go-utils/ternary"
	"github.com/redis/go-redis/v9"

	"github.com/mylxsw/aidea-server/config"
	"github.com/mylxsw/aidea-server/internal/coins"
	"github.com/mylxsw/aidea-server/internal/queue"
	"github.com/mylxsw/aidea-server/server/auth"
	"github.com/mylxsw/aidea-server/server/controllers/common"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"github.com/mylxsw/go-utils/array"
	"github.com/mylxsw/go-utils/str"
)

const (
	AllInOneIslandID = "all-in-one"
)

// CreativeIslandController 创作岛
type CreativeIslandController struct {
	conf         *config.Config
	quotaRepo    *repo.QuotaRepo          `autowire:"@"`
	queue        *queue.Queue             `autowire:"@"`
	trans        youdao.Translater        `autowire:"@"`
	creativeRepo *repo.CreativeRepo       `autowire:"@"`
	securitySrv  *service.SecurityService `autowire:"@"`
	userSvc      *service.UserService     `autowire:"@"`
	rds          *redis.Client            `autowire:"@"`
	xfai         *xfyun.XFYunAI           `autowire:"@"`
}

// NewCreativeIslandController create a new CreativeIslandController
func NewCreativeIslandController(resolver infra.Resolver, conf *config.Config) web.Controller {
	ctl := CreativeIslandController{conf: conf}
	resolver.MustAutoWire(&ctl)
	return &ctl
}

func (ctl *CreativeIslandController) Register(router web.Router) {
	router.Group("/creative", func(router web.Router) {
		router.Get("/items", ctl.Items)
	})

	router.Group("/creative-island", func(router web.Router) {
		router.Get("/capacity", ctl.Capacity)
		router.Get("/models", ctl.Models)
		router.Get("/filters", ctl.ImageStyles)

		router.Group("/histories", func(router web.Router) {
			router.Get("/", ctl.Histories)
			router.Get("/{hid}", ctl.HistoryItem)
			router.Delete("/{hid}", ctl.DeleteHistoryItem)
			router.Post("/{hid}/share", ctl.ShareHistoryItem)
			router.Delete("/{hid}/share", ctl.CancelShareHistoryItem)
		})

		router.Group("/completions", func(router web.Router) {
			// 文生图、图生图
			router.Post("/", ctl.Completions)
			router.Post("/evaluate", ctl.CompletionsEvaluate)
			// 图生视频
			router.Post("/image-to-video", ctl.ImageToVideo)
			// 图片放大
			router.Post("/upscale", ctl.ImageUpscale)
			// 图片上色
			router.Post("/colorize", ctl.ImageColorize)
			// QR 生成、艺术字生成
			router.Post("/artistic-text", ctl.ArtisticText)
		})
	})
}

type CreativeIslandItem struct {
	ID           string `json:"id,omitempty"`
	Title        string `json:"title,omitempty"`
	TitleColor   string `json:"title_color,omitempty"`
	PreviewImage string `json:"preview_image,omitempty"`
	RouteURI     string `json:"route_uri,omitempty"`
	Tag          string `json:"tag,omitempty"`
	Note         string `json:"note,omitempty"`
	Size         string `json:"size,omitempty"`
}

const (
	SizeLarge  = "large"
	SizeMedium = "medium"
)

func (ctl *CreativeIslandController) Items(ctx context.Context, webCtx web.Context, user *auth.UserOptional, client *auth.ClientInfo) web.Response {
	imageCost := int64(coins.GetUnifiedImageGenCoins(""))
	videoCost := int64(coins.GetUnifiedVideoGenCoins("stability-image-to-video"))

	imageModelsCost := coins.GetImageGenCoinsExcept(imageCost)
	imageModelsCostNote := ""
	if len(imageModelsCost) > 0 {
		ns := make([]string, 0)
		for mod, cost := range imageModelsCost {
			ns = append(ns, fmt.Sprintf("%s %d/张", strings.ToUpper(mod), cost))
		}

		if len(ns) > 0 {
			imageModelsCostNote = fmt.Sprintf("（以下模型除外，%s）", strings.Join(ns, "，"))
		}
	}

	items := []CreativeIslandItem{
		{
			ID:           "text-to-image",
			Title:        "文生图",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/image-text-to-image.jpeg-thumb1000",
			RouteURI:     "/creative-draw/create?mode=text-to-image&id=text-to-image",
			Note:         fmt.Sprintf("根据你的想法生成图片。生成每张图片将消耗 %d 智慧果%s。", imageCost, imageModelsCostNote),
			Size:         SizeLarge,
		},
	}

	if client != nil && misc.VersionNewer(client.Version, "1.0.10") && ctl.conf.EnableStabilityAI {
		items = append(items, CreativeIslandItem{
			ID:           "image-to-video",
			Title:        "图生视频",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/image-to-video-dark.jpg-thumb1000",
			RouteURI:     "/creative-draw/create-video",
			Note:         fmt.Sprintf("基于上传的图片再创作，生成一个时长为 2s 的短视频。生成每个视频将消耗 %d 智慧果。", videoCost),
			Size:         SizeLarge,
		})
	}

	enableArtistText := ctl.conf.EnableDashScopeAI && misc.VersionNewer(client.Version, "1.0.11")
	if enableArtistText {
		items = append(items, CreativeIslandItem{
			ID:           "artistic-text",
			Title:        "艺术字",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/artistic-wordart-v2.jpg-thumb1000",
			RouteURI:     "/creative-draw/artistic-wordart?id=artistic-text",
			Note:         fmt.Sprintf("根据你的想法生成图片，并且在图片中融入你写的文字内容。生成每张图片将消耗 %d 智慧果。", imageCost),
			Size:         SizeMedium,
		})
	}

	if client != nil && misc.VersionNewer(client.Version, "1.0.8") && ctl.conf.EnableLeptonAI {
		items = append(items, CreativeIslandItem{
			ID:           "artistic-text",
			Title:        "图文融合",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/artistic-text-v2.jpg-thumb1000",
			RouteURI:     "/creative-draw/artistic-text?type=text&id=artistic-text",
			Note:         fmt.Sprintf("根据你的想法生成图片，并且在图片中融入你写的文字内容。生成每张图片将消耗 %d 智慧果。", imageCost),
			Size:         ternary.If(enableArtistText, SizeMedium, SizeLarge),
		})
		items = append(items, CreativeIslandItem{
			ID:           "artistic-qr",
			Title:        "艺术二维码",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/art-qr-bg.jpg-thumb1000",
			RouteURI:     "/creative-draw/artistic-text?type=qr&id=artistic-qr",
			Note:         fmt.Sprintf("根据你的想法生成图片，并且将链接地址转换为二维码，把图片和二维码融合到一起。生成每张图片将消耗 %d 智慧果。", imageCost),
			Size:         SizeMedium,
		})
	}

	items = append(items, CreativeIslandItem{
		ID:           "image-to-image",
		Title:        "图生图",
		TitleColor:   "FFFFFFFF",
		PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/image-image-to-image.jpeg-thumb1000",
		RouteURI:     "/creative-draw/create?mode=image-to-image&id=image-to-image",
		Tag:          ternary.If(client != nil && client.IsIOS(), "", "BETA"),
		Note:         fmt.Sprintf("基于参考图片的轮廓，为你生成一张整体结构类似的图片。生成每张图片将消耗 %d 智慧果。", imageCost),
		Size:         SizeMedium,
	})

	if client != nil && misc.VersionNewer(client.Version, "1.0.2") && ctl.conf.EnableDeepAI {
		items = append(items, CreativeIslandItem{
			ID:           "image-upscale",
			Title:        "高清修复",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/super-res.jpeg-thumb1000",
			RouteURI:     "/creative-draw/create-upscale",
			Note:         fmt.Sprintf("将低分辨率的照片升级到高分辨率，让图片的清晰度得到明显提升。\n生成每张图片将消耗 %d 智慧果。", imageCost),
			Size:         SizeMedium,
		})

		items = append(items, CreativeIslandItem{
			ID:           "image-colorize",
			Title:        "旧照片上色",
			TitleColor:   "FFFFFFFF",
			PreviewImage: "https://ssl.aicode.cc/ai-server/assets/background/image-colorizev2.jpeg-thumb1000",
			RouteURI:     "/creative-draw/create-colorize",
			Note:         fmt.Sprintf("将黑白照片变成彩色照片，让照片的色彩更加丰富。\n生成每张图片将消耗 %d 智慧果。", imageCost),
			Size:         SizeMedium,
		})
	}

	// 如果中等大小的项目不足 2 个，则把所有的项目都设置为大尺寸
	// TODO 临时处理
	if len(array.Filter(items, func(item CreativeIslandItem, _ int) bool { return item.Size == SizeMedium })) < 2 {
		items = array.Map(items, func(item CreativeIslandItem, _ int) CreativeIslandItem {
			item.Size = SizeLarge
			return item
		})
	}

	return webCtx.JSON(web.M{
		"data": items,
	})
}

type CreativeIslandCapacity struct {
	ShowAIRewrite            bool            `json:"show_ai_rewrite,omitempty"`
	ShowUpscaleBy            bool            `json:"show_upscale_by,omitempty"`
	ShowNegativeText         bool            `json:"show_negative_text,omitempty"`
	ShowStyle                bool            `json:"show_style,omitempty"`
	ShowImageCount           bool            `json:"show_image_count,omitempty"`
	ShowSeed                 bool            `json:"show_seed,omitempty"`
	ShowPromptForImage2Image bool            `json:"show_prompt_for_image2image,omitempty"`
	AllowRatios              []string        `json:"allow_ratios,omitempty"`
	VendorModels             []VendorModel   `json:"vendor_models,omitempty"`
	Filters                  []ImageStyle    `json:"filters,omitempty"`
	AllowUpscaleBy           []string        `json:"allow_upscale_by,omitempty"`
	ShowImageStrength        bool            `json:"show_image_strength,omitempty"`
	ArtisticStyles           []ArtisticStyle `json:"artistic_styles,omitempty"`
	ArtisticTextStyles       []ArtisticStyle `json:"artistic_text_styles,omitempty"`
	ArtisticTextFonts        []ArtisticStyle `json:"artistic_text_fonts,omitempty"`
}

// Models 可用的模型列表
func (ctl *CreativeIslandController) Models(ctx context.Context, webCtx web.Context) web.Response {
	return webCtx.JSON(web.M{
		"data": ctl.loadAllModels(ctx),
	})
}

// loadAllModels 加载所有的模型
// TODO 加缓存
func (ctl *CreativeIslandController) loadAllModels(ctx context.Context) []repo.ImageModel {
	models, err := ctl.creativeRepo.Models(ctx)
	if err != nil {
		log.Errorf("get models failed: %v", err)
	}

	return array.Filter(models, func(m repo.ImageModel, _ int) bool {
		if m.Vendor == "leapai" {
			return ctl.conf.EnableLeapAI
		}

		if m.Vendor == "stabilityai" {
			return ctl.conf.EnableStabilityAI
		}

		if m.Vendor == "fromston" {
			return ctl.conf.EnableFromstonAI
		}

		if m.Vendor == "getimgai" {
			return ctl.conf.EnableGetimgAI
		}

		if m.Vendor == "dashscope" {
			return ctl.conf.EnableDashScopeAI
		}

		if m.Vendor == "dalle" {
			return ctl.conf.EnableOpenAIDalle
		}

		return true
	})
}

// ImageStyles 图片风格，历史遗留问题可能部分代码也是用了 Filter 这个名字
// TODO 加缓存
func (ctl *CreativeIslandController) ImageStyles(ctx context.Context, webCtx web.Context) web.Response {
	filters, err := ctl.creativeRepo.Filters(ctx)
	if err != nil {
		log.Errorf("get filters failed: %v", err)
		return webCtx.JSONError(common.ErrInternalError, http.StatusInternalServerError)
	}

	// 查询所有可用的模型，转换为 map[模型ID]模型ID
	availableModels := array.ToMap(
		array.Map(ctl.loadAllModels(ctx), func(item repo.ImageModel, _ int) string {
			return item.ModelId
		}),
		func(val string, _ int) string {
			return val
		},
	)

	// 过滤掉当前没有启用的模型
	filters = array.Filter(filters, func(item repo.ImageFilter, _ int) bool {
		_, ok := availableModels[item.ModelId]
		return ok
	})

	return webCtx.JSON(web.M{
		"data": filters,
	})
}

// Capacity 文生图、图生图支持的能力，用于控制客户端显示哪些允许用户配置的参数
func (ctl *CreativeIslandController) Capacity(ctx context.Context, webCtx web.Context, user *auth.UserOptional) web.Response {
	mode := webCtx.InputWithDefault("mode", "text-to-image")
	id := webCtx.Input("id")

	log.WithFields(log.Fields{"id": id, "mode": mode}).Debugf("creative capacity request")

	filters := array.Sort(
		array.Filter(ctl.getAllImageStyles(ctx), func(item ImageStyle, index int) bool {
			if !ctl.conf.EnableLeapAI && item.Vendor == "leapai" {
				return false
			}

			if !ctl.conf.EnableStabilityAI && item.Vendor == "stabilityai" {
				return false
			}

			if !ctl.conf.EnableFromstonAI && item.Vendor == "fromston" {
				return false
			}

			if !ctl.conf.EnableGetimgAI && item.Vendor == "getimgai" {
				return false
			}

			if !ctl.conf.EnableOpenAIDalle && item.Vendor == "dalle" {
				return false
			}

			return str.In(mode, item.Supports)
		}),
		func(f1, f2 ImageStyle) bool { return sortorder.NaturalLess(f1.Name, f2.Name) },
	)

	var models []VendorModel
	if user.User != nil && user.User.InternalUser() {
		models = array.Sort(array.Filter(ctl.getAllModels(ctx), func(v VendorModel, _ int) bool { return v.Enabled }), func(v1, v2 VendorModel) bool {
			if v1.Vendor == v2.Vendor {
				return sortorder.NaturalLess(v1.Name, v2.Name)
			}

			return sortorder.NaturalLess(v1.Vendor, v2.Vendor)
		})

		models = array.Map(models, func(item VendorModel, _ int) VendorModel {
			if !user.User.InternalUser() {
				item.Vendor = ""
			}

			return item
		})
	}

	artisticStyle := make([]ArtisticStyle, 0)
	// 艺术字、艺术二维码风格
	if ctl.conf.EnableLeptonAI {
		// "realism", "anime", "comics", "dream", "prime", "2.5d"
		artisticStyle = append(artisticStyle, ArtisticStyle{ID: "realism", Name: "写实", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/art-style-realism.png-avatar"})
		artisticStyle = append(artisticStyle, ArtisticStyle{ID: "anime", Name: "动漫", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/art-style-anime.png-avatar"})
		artisticStyle = append(artisticStyle, ArtisticStyle{ID: "comics", Name: "漫画", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/art-style-comics.png-avatar"})
		artisticStyle = append(artisticStyle, ArtisticStyle{ID: "dream", Name: "梦幻", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/art-style-dream.png-avatar"})
		artisticStyle = append(artisticStyle, ArtisticStyle{ID: "prime", Name: "素描", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/art-style-prime.png-avatar"})
		artisticStyle = append(artisticStyle, ArtisticStyle{ID: "2.5d", Name: "2.5D", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/art-style-2.5d.png-avatar"})
	}

	// 艺术字风格（阿里云锦书接口）
	artisticTextStyle := make([]ArtisticStyle, 0)
	artisticTextFonts := make([]ArtisticStyle, 0)
	if ctl.conf.EnableDashScopeAI {
		artisticTextStyle = append(artisticTextStyle, ArtisticStyle{ID: "material", Name: "立体材质", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/style-material.jpg-avatar"})
		artisticTextStyle = append(artisticTextStyle, ArtisticStyle{ID: "scene", Name: "场景融合", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/style-scene.jpg-avatar"})

		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "dongfangdakai", Name: "阿里妈妈东方大楷", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/阿里妈妈东方大楷.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "puhuiti_m", Name: "阿里巴巴普惠体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/阿里巴巴普惠体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "shuheiti", Name: "阿里妈妈数黑体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/阿里妈妈数黑体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "jinbuti", Name: "钉钉进步体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/钉钉进步体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "kuheiti", Name: "站酷酷黑体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/站酷酷黑体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "kuaileti", Name: "站酷快乐体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/站酷快乐体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "wenyiti", Name: "站酷文艺体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/站酷文艺体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "logoti", Name: "站酷小薇LOGO体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/站酷小薇LOGO体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "cangeryuyangti_m", Name: "站酷仓耳渔阳体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/站酷仓耳渔阳体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "siyuansongti_b", Name: "思源宋体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/思源宋体.jpg-avatar"})
		artisticTextFonts = append(artisticTextFonts, ArtisticStyle{ID: "siyuanheiti_m", Name: "思源黑体", PreviewImage: "https://ssl.aicode.cc/ai-server/assets/styles/fonts/思源黑体.jpg-avatar"})
	}

	return webCtx.JSON(CreativeIslandCapacity{
		ShowAIRewrite:            true,
		ShowUpscaleBy:            true,
		AllowRatios:              []string{"1:1" /*"4:3", "3:4",*/, "3:2", "2:3" /*"16:9"*/},
		ShowStyle:                true,
		ShowNegativeText:         true,
		ShowSeed:                 user.User != nil && user.User.InternalUser(),
		ShowImageCount:           user.User != nil && user.User.InternalUser(),
		ShowPromptForImage2Image: true,
		Filters:                  filters,
		VendorModels:             models,
		AllowUpscaleBy:           []string{"x1", "x2", "x4"},
		ShowImageStrength:        user.User != nil && user.User.InternalUser(),
		ArtisticStyles:           artisticStyle,
		ArtisticTextStyles:       artisticTextStyle,
		ArtisticTextFonts:        artisticTextFonts,
	})
}

// ShareHistoryItem 分享创作到发现页
func (ctl *CreativeIslandController) ShareHistoryItem(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	hid, _ := strconv.Atoi(webCtx.PathVar("hid"))
	if hid <= 0 {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInvalidRequest), http.StatusBadRequest)
	}

	err := ctl.creativeRepo.ShareCreativeHistoryToGallery(ctx, user.ID, user.Name, int64(hid))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrNotFound), http.StatusNotFound)
		}

		log.WithFields(log.Fields{
			"uid":    user.ID,
			"his_id": hid,
		}).Errorf("share creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{})
}

// CancelShareHistoryItem 取消分享创作到发现页
func (ctl *CreativeIslandController) CancelShareHistoryItem(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	hid, _ := strconv.Atoi(webCtx.PathVar("hid"))
	if hid <= 0 {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInvalidRequest), http.StatusBadRequest)
	}

	userID := user.ID
	if user.InternalUser() {
		userID = 0
	}

	err := ctl.creativeRepo.CancelCreativeHistoryShare(ctx, userID, int64(hid))
	if err != nil {
		log.WithFields(log.Fields{
			"uid":    user.ID,
			"his_id": hid,
		}).Errorf("cancel share creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{})
}

// Histories 获取创作岛项目的历史记录
func (ctl *CreativeIslandController) Histories(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	page := webCtx.Int64Input("page", 1)
	if page < 1 || page > 1000 {
		page = 1
	}

	perPage := webCtx.Int64Input("per_page", 20)
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	items, meta, err := ctl.creativeRepo.HistoryRecordPaginate(ctx, user.ID, repo.CreativeHistoryQuery{
		Page:        page,
		PerPage:     perPage,
		IslandId:    AllInOneIslandID,
		IslandModel: webCtx.Input("model"),
	})
	if err != nil {
		log.Errorf("query creative items failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	// 以下字段不需要返回给前端
	items = array.Map(items, func(item repo.CreativeHistoryItem, _ int) repo.CreativeHistoryItem {
		//  Arguments 只保留必须的 image 字段，用于客户端区分是文生图还是图生图
		var arguments map[string]any
		_ = json.Unmarshal([]byte(item.Arguments), &arguments)

		item.Arguments = ""
		if arguments != nil {
			image, ok := arguments["image"]
			if ok {
				data, _ := json.Marshal(map[string]any{"image": image})
				item.Arguments = string(data)
			}
		}

		item.Prompt = ""
		item.QuotaUsed = 0

		switch item.IslandType {
		case int64(repo.IslandTypeImage):
			if arguments != nil {
				if _, ok := arguments["image"]; ok {
					item.IslandTitle = "图生图"
				}
			}

			if item.IslandTitle == "" {
				item.IslandTitle = "文生图"
			}
		case int64(repo.IslandTypeUpscale):
			item.IslandTitle = "高清修复"
		case int64(repo.IslandTypeImageColorization):
			item.IslandTitle = "图片上色"
		case int64(repo.IslandTypeVideo):
			item.IslandTitle = "图生视频"
		case int64(repo.IslandTypeArtisticText):
			item.IslandTitle = "艺术字"
		}

		// 客户端目前不支持封禁状态展示，这里转换为失败
		if item.Status == int64(repo.CreativeStatusForbid) {
			item.Status = int64(repo.CreativeStatusFailed)
		}

		return item
	})

	// TODO 正式发布后，不返回 ImageStyles，这里只是发布前预览
	filters := ctl.getAllImageStyles(ctx)
	filters = array.Map(filters, func(filter ImageStyle, _ int) ImageStyle {
		filter.PreviewImage = ""
		return filter
	})

	return webCtx.JSON(web.M{
		"data":      items,
		"filters":   filters,
		"page":      meta.Page,
		"per_page":  meta.PerPage,
		"total":     meta.Total,
		"last_page": meta.LastPage,
	})
}

type CreativeHistoryItemResp struct {
	repo.CreativeHistoryItem
	ShowBetaFeature bool `json:"show_beta_feature,omitempty"`
}

// HistoryItem 获取创作岛项目的历史记录详情
func (ctl *CreativeIslandController) HistoryItem(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	hid, _ := strconv.Atoi(webCtx.PathVar("hid"))
	if hid <= 0 {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInvalidRequest), http.StatusBadRequest)
	}

	userId := user.ID
	if user.InternalUser() {
		userId = 0
	}

	item, err := ctl.creativeRepo.FindHistoryRecord(ctx, userId, int64(hid))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrNotFound), http.StatusNotFound)
		}

		log.Errorf("query creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	// 客户端目前不支持封禁状态展示，这里转换为失败
	if item.Status == int64(repo.CreativeStatusForbid) {
		item.Status = int64(repo.CreativeStatusFailed)
	}

	return webCtx.JSON(CreativeHistoryItemResp{
		CreativeHistoryItem: *item,
		ShowBetaFeature:     user.InternalUser(),
	})
}

// DeleteHistoryItem 删除创作岛项目的历史记录
func (ctl *CreativeIslandController) DeleteHistoryItem(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	hid, _ := strconv.Atoi(webCtx.PathVar("hid"))
	if hid <= 0 {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInvalidRequest), http.StatusBadRequest)
	}

	log.WithFields(log.Fields{
		"uid":    user.ID,
		"his_id": hid,
	}).Infof("delete creative item")

	if err := ctl.creativeRepo.DeleteHistoryRecord(ctx, user.ID, int64(hid)); err != nil {
		log.Errorf("delete creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{})
}

// CompletionsEvaluate 创作岛项目文本生成 价格评估
func (ctl *CreativeIslandController) CompletionsEvaluate(ctx context.Context, webCtx web.Context, user *auth.User, client *auth.ClientInfo) web.Response {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, errResp := ctl.resolveImageCompletionRequest(ctx, webCtx, user, client)
	if errResp != nil {
		return errResp
	}

	// 检查用户是否有足够的智慧果
	quota, err := ctl.quotaRepo.GetUserQuota(ctx, user.ID)
	if err != nil {
		log.Errorf("get user quota failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	if !user.InternalUser() {
		req.Quota = 0
	}

	return webCtx.JSON(web.M{"cost": req.Quota, "enough": quota.Quota >= quota.Used+req.Quota, "wait_duration": 45})
}

// resolveImageCompletionRequest 解析创作岛项目图片生成请求参数
func (ctl *CreativeIslandController) resolveImageCompletionRequest(ctx context.Context, webCtx web.Context, user *auth.User, client *auth.ClientInfo) (*queue.ImageCompletionPayload, web.Response) {
	image := webCtx.Input("image")
	if image != "" && !str.HasPrefixes(image, []string{"http://", "https://"}) {
		return nil, webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	promptTags := array.Uniq(array.Filter(
		strings.Split(webCtx.Input("prompt_tags"), ","),
		func(tag string, _ int) bool {
			return tag != ""
		},
	))

	prompt := strings.Trim(strings.ReplaceAll(strings.TrimSpace(webCtx.Input("prompt")), "，", ","), ",")
	if prompt == "" && image == "" {
		return nil, webCtx.JSONError("prompt is required", http.StatusBadRequest)
	}

	negativePrompt := strings.ReplaceAll(strings.TrimSpace(webCtx.Input("negative_prompt")), "，", ",")
	if misc.WordCount(negativePrompt) > 1000 {
		return nil, webCtx.JSONError(fmt.Sprintf("排除内容输入字数不能超过 %d", 1000), http.StatusBadRequest)
	}

	imageCount := webCtx.Int64Input("image_count", 1)
	if imageCount < 1 || imageCount > 4 {
		return nil, webCtx.JSONError("invalid image count", http.StatusBadRequest)
	}

	steps := webCtx.IntInput("steps", 30)
	if !array.In(steps, []int{30, 50, 100, 150}) {
		return nil, webCtx.JSONError("invalid steps", http.StatusBadRequest)
	}

	// AI 自动改写
	aiRewrite := webCtx.InputWithDefault("ai_rewrite", "false") == "true"
	// 图生图模式，不启用 AI 改写
	if image != "" {
		aiRewrite = false
	}

	upscaleBy := webCtx.InputWithDefault("upscale_by", "x1")
	if !array.In(upscaleBy, []string{"x1", "x2", "x4"}) {
		return nil, webCtx.JSONError("invalid upscale_by", http.StatusBadRequest)
	}

	stylePreset := webCtx.Input("style_preset")

	modelID := webCtx.InputWithDefault(
		"model",
		ternary.If(image != "", ctl.conf.DefaultImageToImageModel, ctl.conf.DefaultTextToImageModel),
	)
	filterID := webCtx.Int64Input("filter_id", 0)
	var filterName, defaultFilterMode string
	if filterID > 0 {
		filter := ctl.getStyleByID(ctx, filterID)
		if filter == nil {
			return nil, webCtx.JSONError("invalid filter_id", http.StatusBadRequest)
		}

		modelID = filter.ModelID
		filterName = filter.Name
		defaultFilterMode = filter.Mode
	} else {
		// 如果没有指定 filter， 则自动根据模型补充 filter 信息
		mode := ternary.If(image != "", "image-to-image", "text-to-image")
		filter := ctl.getStyleByModelID(ctx, modelID, mode)
		if filter != nil {
			filterID = filter.ID
			filterName = filter.Name
			defaultFilterMode = filter.Mode
		}
	}

	vendorModel := ctl.getVendorModel(ctx, modelID)
	if vendorModel == nil {
		return nil, webCtx.JSONError("没有找到匹配的模型", http.StatusBadRequest)
	}

	imageRatio := webCtx.InputWithDefault("image_ratio", "1:1")
	if !array.In(imageRatio, []string{"1:1", "4:3", "3:4", "3:2", "2:3", "16:9"}) {
		return nil, webCtx.JSONError("invalid image ratio", http.StatusBadRequest)
	}

	// 图生图模式下有效（ControlNet）
	if defaultFilterMode == "" {
		defaultFilterMode = "canny"
	}

	mode := webCtx.InputWithDefault("mode", defaultFilterMode)
	if !array.In(mode, []string{"canny", "mlsd", "pose", "scribble"}) {
		mode = defaultFilterMode
	}

	// 根据模型配置，自动调整相关参数（width/height）
	dimension := vendorModel.GetDimension(imageRatio)

	width, height := webCtx.IntInput("width", dimension.Width), webCtx.IntInput("height", dimension.Height)
	if width < 1 || height < 1 || width > 2048 || height > 2048 {
		return nil, webCtx.JSONError("invalid width or height", http.StatusBadRequest)
	}

	imageStrength := webCtx.Float64Input("image_strength", 0.65)
	if imageStrength < 0 || imageStrength > 1 {
		return nil, webCtx.JSONError("invalid image_strength", http.StatusBadRequest)
	}

	if imageStrength == 0 {
		imageStrength = 0.65
	}

	// TODO 临时处理：0.5 效果不明显，但是客户端默认为 0.5，需要客户端同步调整
	if imageStrength == 0.5 && misc.VersionOlder(client.Version, "1.0.7") {
		imageStrength = 0.65
	}

	seed := webCtx.Int64Input("seed", int64(rand.Intn(2147483647)))
	if seed < 0 || seed > 2147483647 {
		return nil, webCtx.JSONError("invalid seed", http.StatusBadRequest)
	}

	return &queue.ImageCompletionPayload{
		Prompt:         prompt,
		NegativePrompt: negativePrompt,
		PromptTags:     promptTags,
		ImageCount:     imageCount,
		ImageRatio:     imageRatio,
		Width:          int64(width),
		Height:         int64(height),
		Steps:          int64(steps),
		Image:          image,
		AIRewrite:      aiRewrite,
		Mode:           mode,
		UpscaleBy:      upscaleBy,
		StylePreset:    stylePreset,
		Seed:           seed,
		ImageStrength:  imageStrength,
		FilterID:       filterID,
		FilterName:     filterName,
		GalleryCopyID:  webCtx.Int64Input("gallery_copy_id", 0),

		UID:       user.ID,
		Quota:     int64(coins.GetUnifiedImageGenCoins(vendorModel.Model)) * imageCount,
		CreatedAt: time.Now(),

		Vendor:    vendorModel.Vendor,
		Model:     vendorModel.Model,
		ModelName: vendorModel.Name,
	}, nil
}

func (ctl *CreativeIslandController) getAllModels(ctx context.Context) []VendorModel {
	return array.Map(ctl.loadAllModels(ctx), func(m repo.ImageModel, _ int) VendorModel {
		return VendorModel{
			ID:                m.ModelId,
			Name:              m.ModelName,
			Vendor:            m.Vendor,
			Model:             m.RealModel,
			Enabled:           m.Status == 1,
			Upscale:           m.ImageMeta.Upscale,
			ShowStyle:         m.ImageMeta.ShowStyle,
			ShowImageStrength: m.ImageMeta.ShowImageStrength,
			IntroURL:          m.ImageMeta.IntroURL,
			RatioDimensions:   m.ImageMeta.RatioDimensions,
		}
	})
}

func (ctl *CreativeIslandController) getVendorModel(ctx context.Context, modelID string) *VendorModel {
	models := ctl.getAllModels(ctx)
	for _, m := range models {
		if m.ID == modelID {
			return &m
		}
	}

	return nil
}

type ArtisticStyle struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PreviewImage string `json:"preview_image"`
}

type ImageStyle struct {
	ID             int64    `json:"id,omitempty"`
	Name           string   `json:"name,omitempty"`
	PreviewImage   string   `json:"preview_image,omitempty"`
	Description    string   `json:"description,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	ModelID        string   `json:"-"`
	Vendor         string   `json:"-"`
	Prompt         string   `json:"-"`
	NegativePrompt string   `json:"-"`
	Supports       []string `json:"-"`
}

func (ctl *CreativeIslandController) getAllImageStyles(ctx context.Context) []ImageStyle {
	filters, err := ctl.creativeRepo.Filters(ctx)
	if err != nil {
		log.Errorf("get filters failed: %v", err)
		return []ImageStyle{}
	}

	return array.Map(filters, func(f repo.ImageFilter, _ int) ImageStyle {
		return ImageStyle{
			ID:             f.Id,
			Name:           f.Name,
			PreviewImage:   f.PreviewImage,
			Description:    f.Description,
			ModelID:        f.ModelId,
			Mode:           f.ImageMeta.Mode,
			Prompt:         f.ImageMeta.Prompt,
			NegativePrompt: f.ImageMeta.NegativePrompt,
			Supports:       f.ImageMeta.Supports,
			Vendor:         f.Vendor,
		}
	})
}

func (ctl *CreativeIslandController) getStyleByID(ctx context.Context, styleID int64) *ImageStyle {
	filters := ctl.getAllImageStyles(ctx)
	if len(filters) == 0 {
		return nil
	}

	for _, f := range filters {
		if f.ID == styleID {
			return &f
		}
	}

	return nil
}

func (ctl *CreativeIslandController) getStyleByModelID(ctx context.Context, modelID string, mode string) *ImageStyle {
	filters := ctl.getAllImageStyles(ctx)
	if len(filters) == 0 {
		return nil
	}

	if len(filters) == 1 {
		return &filters[0]
	}

	matched := array.Filter(filters, func(item ImageStyle, _ int) bool {
		return item.ModelID == modelID && array.In(mode, item.Supports)
	})

	if len(matched) == 1 {
		return &matched[0]
	}

	return nil
}

type VendorModel struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Vendor            string                    `json:"vendor,omitempty"`
	Model             string                    `json:"-"`
	Enabled           bool                      `json:"-"`
	Upscale           bool                      `json:"upscale,omitempty"`
	ShowStyle         bool                      `json:"show_style,omitempty"`
	ShowImageStrength bool                      `json:"show_image_strength,omitempty"`
	IntroURL          string                    `json:"intro_url,omitempty"`
	RatioDimensions   map[string]repo.Dimension `json:"-"`
}

func (vm VendorModel) defaultDimension(ratio string) repo.Dimension {
	switch ratio {
	case "1:1":
		return repo.Dimension{Width: 512, Height: 512}
	case "4:3":
		return repo.Dimension{Width: 768, Height: 576}
	case "3:4":
		return repo.Dimension{Width: 576, Height: 768}
	case "3:2":
		return repo.Dimension{Width: 768, Height: 512}
	case "2:3":
		return repo.Dimension{Width: 512, Height: 768}
	case "16:9":
		return repo.Dimension{Width: 1024, Height: 576}
	}

	return repo.Dimension{Width: 512, Height: 512}
}

func (vm VendorModel) GetDimension(ratio string) repo.Dimension {
	if vm.RatioDimensions == nil {
		vm.RatioDimensions = map[string]repo.Dimension{}
	}

	dimension, ok := vm.RatioDimensions[ratio]
	if !ok {
		return vm.defaultDimension(ratio)
	}

	if dimension.Width == 0 || dimension.Height == 0 {
		def := vm.defaultDimension(ratio)
		if dimension.Width <= 0 {
			dimension.Width = def.Width
		}

		if dimension.Height <= 0 {
			dimension.Height = def.Height
		}
	}

	return dimension
}

func (ctl *CreativeIslandController) ImageUpscale(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	image := webCtx.Input("image")
	if image != "" && !str.HasPrefixes(image, []string{"http://", "https://"}) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	// 图片地址检查
	if !strings.HasPrefix(image, ctl.conf.StorageDomain) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	// 检查用户是否有足够的智慧果
	quota, err := ctl.userSvc.UserQuota(ctx, user.ID)
	if err != nil {
		log.Errorf("get user quota failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	quotaConsume := int64(coins.GetUnifiedImageGenCoins(""))
	if quota.Rest-quota.Freezed < quotaConsume {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrQuotaNotEnough), http.StatusPaymentRequired)
	}

	upscaleBy := "x4"

	req := queue.ImageUpscalePayload{
		UserID:    user.ID,
		Image:     image,
		UpscaleBy: upscaleBy,
		Quota:     quotaConsume,
		CreatedAt: time.Now(),
	}

	// 加入异步任务队列
	taskID, err := ctl.queue.Enqueue(&req, queue.NewImageUpscaleTask)
	if err != nil {
		log.Errorf("enqueue task failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}
	log.WithFields(log.Fields{"task_id": taskID}).Debugf("enqueue task success: %s", taskID)

	// 冻结智慧果
	if err := ctl.userSvc.FreezeUserQuota(ctx, user.ID, req.Quota); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛冻结用户配额失败: %s", err)
	}

	if err := ctl.rds.SetEx(ctx, fmt.Sprintf("creative-island:%d:task:%s:quota-freeze", user.ID, taskID), req.Quota, 5*time.Minute).Err(); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛用户配额已冻结，更新 Redis 任务与配额关系失败: %s", err)
	}

	creativeItem := repo.CreativeItem{
		IslandId:   AllInOneIslandID,
		IslandType: repo.IslandTypeUpscale,
		TaskId:     taskID,
		Status:     repo.CreativeStatusPending,
	}

	arg := repo.CreativeRecordArguments{
		Image:     image,
		UpscaleBy: upscaleBy,
	}

	// 保存历史记录
	if _, err := ctl.creativeRepo.CreateRecordWithArguments(ctx, user.ID, &creativeItem, &arg); err != nil {
		log.Errorf("create creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{
		"task_id": taskID, // 任务 ID
		"wait":    60,     // 等待时间
	})
}

func (ctl *CreativeIslandController) ImageColorize(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	image := webCtx.Input("image")
	if image != "" && !str.HasPrefixes(image, []string{"http://", "https://"}) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	// 图片地址检查
	if !strings.HasPrefix(image, ctl.conf.StorageDomain) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	// 检查用户是否有足够的智慧果
	quota, err := ctl.userSvc.UserQuota(ctx, user.ID)
	if err != nil {
		log.Errorf("get user quota failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	quotaConsume := int64(coins.GetUnifiedImageGenCoins(""))
	if quota.Rest-quota.Freezed < quotaConsume {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrQuotaNotEnough), http.StatusPaymentRequired)
	}

	req := queue.ImageColorizationPayload{
		UserID:    user.ID,
		Image:     image,
		Quota:     quotaConsume,
		CreatedAt: time.Now(),
	}

	// 加入异步任务队列
	taskID, err := ctl.queue.Enqueue(&req, queue.NewImageColorizationTask)
	if err != nil {
		log.Errorf("enqueue task failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}
	log.WithFields(log.Fields{"task_id": taskID}).Debugf("enqueue task success: %s", taskID)

	// 冻结智慧果
	if err := ctl.userSvc.FreezeUserQuota(ctx, user.ID, req.Quota); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛冻结用户配额失败: %s", err)
	}

	if err := ctl.rds.SetEx(ctx, fmt.Sprintf("creative-island:%d:task:%s:quota-freeze", user.ID, taskID), req.Quota, 5*time.Minute).Err(); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛用户配额已冻结，更新 Redis 任务与配额关系失败: %s", err)
	}

	creativeItem := repo.CreativeItem{
		IslandId:   AllInOneIslandID,
		IslandType: repo.IslandTypeImageColorization,
		TaskId:     taskID,
		Status:     repo.CreativeStatusPending,
	}

	arg := repo.CreativeRecordArguments{
		Image: image,
	}

	// 保存历史记录
	if _, err := ctl.creativeRepo.CreateRecordWithArguments(ctx, user.ID, &creativeItem, &arg); err != nil {
		log.Errorf("create creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{
		"task_id": taskID, // 任务 ID
		"wait":    60,     // 等待时间
	})
}

// ArtisticText 艺术字、QR 生成
// 请求参数：
// - text
// - type: qr/text/word_art（锦书）
// - prompt
// - negative_prompt
// - style_preset
func (ctl *CreativeIslandController) ArtisticText(ctx context.Context, webCtx web.Context, user *auth.User, client *auth.ClientInfo) web.Response {
	text := webCtx.Input("text")
	if text == "" {
		return webCtx.JSONError("invalid text", http.StatusBadRequest)
	}

	optType := webCtx.Input("type")
	if !str.In(optType, []string{"qr", "text", "word_art"}) {
		return webCtx.JSONError("invalid type", http.StatusBadRequest)
	}

	prompt := misc.WordTruncate(webCtx.Input("prompt"), 500)
	if prompt == "" {
		return webCtx.JSONError("invalid prompt", http.StatusBadRequest)
	}

	negativePrompt := misc.WordTruncate(webCtx.Input("negative_prompt"), 500)

	stylePreset := webCtx.Input("style_preset")
	if stylePreset == "" {
		if optType == "word_art" {
			stylePreset = "material"
		} else {
			stylePreset = "realism"
		}
	}

	if !str.In(stylePreset, []string{"realism", "anime", "comics", "dream", "prime", "2.5d" /* 分割线，后面的是锦书 */, "material", "scene"}) {
		return webCtx.JSONError("invalid stylePreset", http.StatusBadRequest)
	}

	// 字体：使用阿里云的锦书服务时有效
	fontName := webCtx.Input("font_name")
	if fontName != "" && !str.In(fontName, []string{"dongfangdakai", "puhuiti_m", "shuheiti", "jinbuti", "kuheiti", "kuaileti", "wenyiti", "logoti", "cangeryuyangti_m", "siyuansongti_b", "siyuanheiti_m", "fangzhengkaiti"}) {
		return webCtx.JSONError("invalid fontName", http.StatusBadRequest)
	}

	imageCount := webCtx.Int64Input("image_count", 1)
	if imageCount < 1 || imageCount > 4 {
		return webCtx.JSONError("invalid image count", http.StatusBadRequest)
	}

	steps := webCtx.IntInput("steps", 30)
	if !array.In(steps, []int{30, 50, 100, 150}) {
		return webCtx.JSONError("invalid steps", http.StatusBadRequest)
	}

	// 检查用户是否有足够的智慧果
	quota, err := ctl.userSvc.UserQuota(ctx, user.ID)
	if err != nil {
		log.Errorf("get user quota failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	quotaConsume := int64(coins.GetUnifiedImageGenCoins("")) * imageCount
	if quota.Rest-quota.Freezed < quotaConsume {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrQuotaNotEnough), http.StatusPaymentRequired)
	}

	controlWeight := webCtx.Float64Input("control_weight", 1.35)
	if controlWeight < 0.1 || controlWeight > 3 {
		return webCtx.JSONError("invalid control_weight", http.StatusBadRequest)
	}

	controlImageRatio := webCtx.Float64Input("control_image_ratio", 0.8)
	if controlImageRatio < 0.1 || controlImageRatio > 1 {
		return webCtx.JSONError("invalid control_image_ratio", http.StatusBadRequest)
	}

	seed := webCtx.Int64Input("seed", -1)
	if seed < 0 || seed > 2147483647 {
		seed = -1
	}

	var req queue.Payload
	var taskBuilder queue.TaskBuilder

	if optType == "word_art" {
		req = &queue.DashscopeImageCompletionPayload{
			Quota:     quotaConsume,
			CreatedAt: time.Now(),

			TextureText:     text,
			Prompt:          prompt,
			TextureStyle:    stylePreset,
			UID:             user.ID,
			FreezedCoins:    quotaConsume,
			TextureFontName: fontName,

			ImageCount: imageCount,
			Steps:      int64(steps),
			Seed:       seed,

			Model: dashscope.WordArtTextureModel,
		}
		taskBuilder = queue.NewDashscopeImageCompletionTask
	} else {
		req = &queue.ArtisticTextCompletionPayload{
			Quota:     quotaConsume,
			CreatedAt: time.Now(),

			Text:           text,
			Type:           optType,
			ArtisticType:   stylePreset,
			Prompt:         prompt,
			NegativePrompt: negativePrompt,
			AIRewrite:      webCtx.Input("ai_rewrite") == "true",
			UID:            user.ID,
			FreezedCoins:   quotaConsume,

			ControlImageRatio: controlImageRatio,
			ControlWeight:     controlWeight,
			GuidanceStart:     0.3,
			GuidanceEnd:       0.95,
			Seed:              seed,
			Steps:             int64(steps),
			CfgScale:          7,
			NumImages:         imageCount,

			FontPath: ctl.conf.FontPath,
		}
		taskBuilder = queue.NewArtisticTextCompletionTask

	}

	// 加入异步任务队列
	taskID, err := ctl.queue.Enqueue(req, taskBuilder)
	if err != nil {
		log.Errorf("enqueue task failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}
	log.WithFields(log.Fields{"task_id": taskID}).Debugf("enqueue task success: %s", taskID)

	// 冻结智慧果
	if err := ctl.userSvc.FreezeUserQuota(ctx, user.ID, req.GetQuota()); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.GetQuota(), "task_id": taskID}).Errorf("创作岛冻结用户配额失败: %s", err)
	}

	if err := ctl.rds.SetEx(ctx, fmt.Sprintf("creative-island:%d:task:%s:quota-freeze", user.ID, taskID), req.GetQuota(), 5*time.Minute).Err(); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.GetQuota(), "task_id": taskID}).Errorf("创作岛用户配额已冻结，更新 Redis 任务与配额关系失败: %s", err)
	}

	creativeItem := repo.CreativeItem{
		IslandId:   AllInOneIslandID,
		IslandType: repo.IslandTypeArtisticText,
		TaskId:     taskID,
		Status:     repo.CreativeStatusPending,
		Prompt:     prompt,
	}

	arg := repo.CreativeRecordArguments{
		NegativePrompt: negativePrompt,
		ArtisticType:   optType,
		StylePreset:    stylePreset,
		Text:           text,
	}

	// 保存历史记录
	if _, err := ctl.creativeRepo.CreateRecordWithArguments(ctx, user.ID, &creativeItem, &arg); err != nil {
		log.Errorf("create creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{
		"task_id": taskID, // 任务 ID
		"wait":    30,     // 等待时间
	})
}

// Completions 创作岛项目文本生成
func (ctl *CreativeIslandController) Completions(ctx context.Context, webCtx web.Context, user *auth.User, client *auth.ClientInfo) web.Response {
	req, errResp := ctl.resolveImageCompletionRequest(ctx, webCtx, user, client)
	if errResp != nil {
		return errResp
	}

	// 图片地址检查
	if req.Image != "" && !str.HasPrefixes(req.Image, []string{"https://ssl.aicode.cc/", ctl.conf.StorageDomain}) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	// stabilityai 和 fromston 生成的图片为正方形
	if req.Image != "" && array.In(req.Vendor, []string{"fromston", "stabilityai"}) {
		req.Image = uploader.BuildImageURLWithFilter(req.Image, "fix_square_1024", ctl.conf.StorageDomain)
	}

	// 检查用户是否有足够的智慧果
	quota, err := ctl.userSvc.UserQuota(ctx, user.ID)
	if err != nil {
		log.Errorf("get user quota failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	if quota.Rest-quota.Freezed < req.Quota {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrQuotaNotEnough), http.StatusPaymentRequired)
	}

	// 内容安全检测
	if checkRes := ctl.securitySrv.PromptDetect(req.Prompt); checkRes != nil {
		if checkRes.IsReallyUnSafe() {
			log.WithFields(log.Fields{
				"user_id": user.ID,
				"details": checkRes.ReasonDetail(),
				"content": req.Prompt,
			}).Errorf("用户 %d 违规，违规内容：%s", user.ID, checkRes.Reason)
			return webCtx.JSONError(fmt.Sprintf("内容违规，已被系统拦截，如有疑问邮件联系：support@aicode.cc\n\n原因：%s", checkRes.ReasonDetail()), http.StatusNotAcceptable)
		}
	}

	// 加入异步任务队列
	taskID, err := ctl.queue.Enqueue(req, queue.NewImageCompletionTask)
	if err != nil {
		log.Errorf("enqueue task failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}
	log.WithFields(log.Fields{"task_id": taskID}).Debugf("enqueue task success: %s", taskID)

	// 冻结智慧果
	if err := ctl.userSvc.FreezeUserQuota(ctx, user.ID, req.Quota); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛冻结用户配额失败: %s", err)
	}

	if err := ctl.rds.SetEx(ctx, fmt.Sprintf("creative-island:%d:task:%s:quota-freeze", user.ID, taskID), req.Quota, 5*time.Minute).Err(); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛用户配额已冻结，更新 Redis 任务与配额关系失败: %s", err)
	}

	// 保存历史记录
	creativeItem, arg := ctl.buildHistorySaveRecord(req, taskID)
	if _, err := ctl.creativeRepo.CreateRecordWithArguments(ctx, user.ID, &creativeItem, &arg); err != nil {
		log.Errorf("create creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{
		"task_id": taskID, // 任务 ID
		"wait":    60,     // 等待时间
	})
}

// buildHistorySaveRecord 构建保存历史记录的 CreativeItem
func (*CreativeIslandController) buildHistorySaveRecord(req *queue.ImageCompletionPayload, taskID string) (repo.CreativeItem, repo.CreativeRecordArguments) {
	creativeItem := repo.CreativeItem{
		IslandId:    AllInOneIslandID,
		IslandType:  repo.IslandTypeImage,
		IslandModel: req.Model,
		Prompt:      req.Prompt,
		TaskId:      taskID,
		Status:      repo.CreativeStatusPending,
	}
	return creativeItem, repo.CreativeRecordArguments{
		NegativePrompt: req.NegativePrompt,
		PromptTags:     req.PromptTags,
		Width:          req.Width,
		Height:         req.Height,
		Steps:          req.Steps,
		ImageCount:     req.ImageCount,
		ImageRatio:     req.ImageRatio,
		StylePreset:    req.StylePreset,
		Mode:           req.Mode,
		Image:          req.Image,
		UpscaleBy:      req.UpscaleBy,
		AIRewrite:      req.AIRewrite,
		ModelID:        req.GetModel(),
		ModelName:      req.ModelName,
		FilterID:       req.FilterID,
		FilterName:     req.FilterName,
		GalleryCopyID:  req.GalleryCopyID,
		Seed:           req.Seed,
	}
}

// ImageToVideo 图片生成视频
// 请求参数：
// - image 图片上传后的地址
// - seed 随机种子
func (ctl *CreativeIslandController) ImageToVideo(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	image := webCtx.Input("image")
	if image != "" && !str.HasPrefixes(image, []string{"http://", "https://"}) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	// 图片地址检查
	if !strings.HasPrefix(image, ctl.conf.StorageDomain) {
		return webCtx.JSONError("invalid image", http.StatusBadRequest)
	}

	width, height := int64(1024), int64(576)

	// 查询图片信息
	info, err := uploader.QueryImageInfo(image)
	if err == nil {
		if info.Width == info.Height {
			width, height = 768, 768
		} else if info.Width > info.Height {
			width, height = 1024, 576
		} else {
			width, height = 576, 1024
		}
	}

	image = uploader.BuildImageURLWithFilter(image, fmt.Sprintf("resize%dx%d", width, height), ctl.conf.StorageDomain)

	// 检查用户是否有足够的智慧果
	quota, err := ctl.userSvc.UserQuota(ctx, user.ID)
	if err != nil {
		log.Errorf("get user quota failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	quotaConsume := int64(coins.GetUnifiedVideoGenCoins(""))
	if quota.Rest-quota.Freezed < quotaConsume {
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrQuotaNotEnough), http.StatusPaymentRequired)
	}

	seed := webCtx.Int64Input("seed", -1)
	if seed < 0 || seed > 2147483647 {
		seed = -1
	}

	// How strongly the video sticks to the original image.
	// Use lower values to allow the model more freedom to make changes and higher values to correct motion distortions.
	cfgScale := webCtx.Float64Input("cfg_scale", 2.5)
	if cfgScale < 1 || cfgScale > 10 {
		cfgScale = 2.5
	}

	// Lower values generally result in less motion in the output video,
	// while higher values generally result in more motion
	motionBucketID := webCtx.IntInput("motion_bucket_id", 40)
	if motionBucketID < 1 || motionBucketID > 255 {
		motionBucketID = 40
	}

	req := queue.ImageToVideoCompletionPayload{
		Quota:          quotaConsume,
		CreatedAt:      time.Now(),
		Image:          image,
		UID:            user.ID,
		Seed:           seed,
		CfgScale:       cfgScale,
		MotionBucketID: motionBucketID,
		Width:          width,
		Height:         height,
	}

	// 加入异步任务队列
	taskID, err := ctl.queue.Enqueue(&req, queue.NewImageToVideoCompletionTask)
	if err != nil {
		log.Errorf("enqueue task failed: %s", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}
	log.WithFields(log.Fields{"task_id": taskID}).Debugf("enqueue task success: %s", taskID)

	// 冻结智慧果
	if err := ctl.userSvc.FreezeUserQuota(ctx, user.ID, req.Quota); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛冻结用户配额失败: %s", err)
	}

	if err := ctl.rds.SetEx(ctx, fmt.Sprintf("creative-island:%d:task:%s:quota-freeze", user.ID, taskID), req.Quota, 5*time.Minute).Err(); err != nil {
		log.F(log.M{"user_id": user.ID, "quota": req.Quota, "task_id": taskID}).Errorf("创作岛用户配额已冻结，更新 Redis 任务与配额关系失败: %s", err)
	}

	creativeItem := repo.CreativeItem{
		IslandId:   AllInOneIslandID,
		IslandType: repo.IslandTypeVideo,
		TaskId:     taskID,
		Status:     repo.CreativeStatusPending,
	}

	arg := repo.CreativeRecordArguments{
		Image:          image,
		Width:          width,
		Height:         height,
		Seed:           seed,
		MotionBucketID: motionBucketID,
		CfgScale:       cfgScale,
	}

	// 保存历史记录
	if _, err := ctl.creativeRepo.CreateRecordWithArguments(ctx, user.ID, &creativeItem, &arg); err != nil {
		log.Errorf("create creative item failed: %v", err)
		return webCtx.JSONError(common.Text(webCtx, ctl.trans, common.ErrInternalError), http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{
		"task_id": taskID, // 任务 ID
		"wait":    30,     // 等待时间
	})
}
