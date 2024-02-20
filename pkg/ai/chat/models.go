package chat

import (
	"github.com/mylxsw/aidea-server/pkg/ai/anthropic"
	"github.com/mylxsw/aidea-server/pkg/ai/baichuan"
	"github.com/mylxsw/aidea-server/pkg/ai/baidu"
	"github.com/mylxsw/aidea-server/pkg/ai/dashscope"
	"github.com/mylxsw/aidea-server/pkg/ai/google"
	"github.com/mylxsw/aidea-server/pkg/ai/gpt360"
	"github.com/mylxsw/aidea-server/pkg/ai/sensenova"
	"github.com/mylxsw/aidea-server/pkg/ai/tencentai"
	"github.com/mylxsw/aidea-server/pkg/ai/xfyun"
	"github.com/mylxsw/go-utils/str"
	"strings"

	"github.com/mylxsw/aidea-server/config"
	"github.com/mylxsw/go-utils/array"
)

type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Category    string `json:"category"`
	IsImage     bool   `json:"is_image"`
	Disabled    bool   `json:"disabled"`
	VersionMin  string `json:"version_min,omitempty"`
	VersionMax  string `json:"version_max,omitempty"`
	Tag         string `json:"tag,omitempty"`

	IsChat        bool `json:"is_chat"`
	SupportVision bool `json:"support_vision,omitempty"`
}

func (m Model) RealID() string {
	segs := strings.SplitN(m.ID, ":", 2)
	return segs[1]
}

func (m Model) IsSensitiveModel() bool {
	return m.Category == "openai" || m.Category == "Anthropic" || m.Category == "google"
}

func (m Model) IsVirtualModel() bool {
	return m.Category == "virtual"
}

func Models(conf *config.Config, returnAll bool) []Model {
	var models []Model
	models = append(models, openAIModels(conf)...)
	models = append(models, anthropicModels(conf)...)
	models = append(models, googleModels(conf)...)
	models = append(models, chinaModels(conf)...)
	models = append(models, aideaModels(conf)...)

	return array.Filter(
		array.Map(models, func(item Model, _ int) Model {
			if item.ShortName == "" {
				item.ShortName = item.Name
			}

			return item
		}),
		func(item Model, _ int) bool {
			if returnAll {
				return true
			}

			return !item.Disabled
		},
	)
}

func openAIModels(conf *config.Config) []Model {
	return []Model{
		{
			ID:          "openai:gpt-3.5-turbo",
			Name:        "GPT-3.5",
			Description: "速度快，成本低",
			Category:    "openai",
			IsChat:      true,
			Disabled:    !conf.EnableOpenAI,
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gpt35.png",
		},
		{
			ID:          "openai:gpt-3.5-turbo-16k",
			Name:        "GPT-3.5 16K",
			Description: "3.5 升级版，支持 16K 长文本",
			Category:    "openai",
			IsChat:      true,
			Disabled:    !conf.EnableOpenAI,
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gpt35.png",
		},
		{
			ID:          "openai:gpt-4",
			Name:        "GPT-4",
			Description: "能力强，更精准",
			Category:    "openai",
			IsChat:      true,
			Disabled:    !conf.EnableOpenAI,
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gpt4.png",
		},
		{
			ID:          "openai:gpt-4-1106-preview",
			Name:        "GPT-4 Turbo",
			ShortName:   "GPT-4 Turbo",
			Description: "能力强，更精准",
			Category:    "openai",
			IsChat:      true,
			Disabled:    !conf.EnableOpenAI,
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gpt4-preview.png",
		},
		{
			ID:            "openai:gpt-4-vision-preview",
			Name:          "GPT-4V（视觉）",
			ShortName:     "GPT-4V",
			Description:   "拥有视觉能力",
			Category:      "openai",
			IsChat:        true,
			SupportVision: true,
			Disabled:      !conf.EnableOpenAI,
			AvatarURL:     "https://ssl.aicode.cc/ai-server/assets/avatar/gpt4-preview.png",
			VersionMin:    "1.0.8",
		},
		{
			ID:          "openai:gpt-4-32k",
			Name:        "GPT-4 32k",
			Description: "基于 GPT-4，但是支持4倍的内容长度",
			Category:    "openai",
			IsChat:      true,
			Disabled:    true,
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gpt4.png",
		},
	}
}

func chinaModels(conf *config.Config) []Model {
	models := make([]Model, 0)

	models = append(models, Model{
		ID:          "讯飞星火:" + string(xfyun.ModelGeneralV1_5),
		Name:        "星火大模型 v1.5",
		ShortName:   "星火 1.5",
		Description: "科大讯飞研发的认知大模型，支持语言理解、知识问答、代码编写、逻辑推理、数学解题等多元能力，服务已内嵌联网搜索功能",
		Category:    "讯飞星火",
		IsChat:      true,
		Disabled:    !conf.EnableXFYunAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/xfyun-v1.5.png",
	})

	models = append(models, Model{
		ID:          "讯飞星火:" + string(xfyun.ModelGeneralV2),
		Name:        "星火大模型 v2.0",
		ShortName:   "星火 2.0",
		Description: "科大讯飞研发的认知大模型，V2.0 在 V1.5 基础上全面升级，并在代码、数学场景进行专项升级，服务已内嵌联网搜索、日期查询、天气查询、股票查询、诗词查询、字词理解等功能",
		Category:    "讯飞星火",
		IsChat:      true,
		Disabled:    !conf.EnableXFYunAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/xfyun-v2.png",
	})

	models = append(models, Model{
		ID:          "讯飞星火:" + string(xfyun.ModelGeneralV3),
		Name:        "星火大模型 v3.0",
		ShortName:   "星火 3.0",
		Description: "科大讯飞研发的认知大模型，V3.0 能力全面升级，在数学、代码、医疗、教育等场景进行了专项优化，让大模型更懂你所需",
		Category:    "讯飞星火",
		IsChat:      true,
		Disabled:    !conf.EnableXFYunAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/xfyun-v3.png",
	})

	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelErnieBotTurbo,
		Name:        "文心一言 Turbo",
		ShortName:   "文心 Turbo",
		Description: "百度研发的知识增强大语言模型，中文名是文心一言，英文名是 ERNIE Bot，能够与人对话互动，回答问题，协助创作，高效便捷地帮助人们获取信息、知识和灵感",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/wenxinyiyan-turbo.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + string(baidu.ModelErnieBot),
		Name:        "文心一言",
		Description: "百度研发的知识增强大语言模型增强版，中文名是文心一言，英文名是 ERNIE Bot，能够与人对话互动，回答问题，协助创作，高效便捷地帮助人们获取信息、知识和灵感",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/creative/wenxinyiyan.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + string(baidu.ModelErnieBot4),
		Name:        "文心一言 4.0",
		ShortName:   "文心 4.0",
		Description: "ERNIE-Bot-4 是百度自行研发的大语言模型，覆盖海量中文数据，具有更强的对话问答、内容创作生成等能力",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.5",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/wenxinyiyan-4.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelLlama2_70b,
		Name:        "Llama 2 70B 英文版",
		ShortName:   "Llama2 70B",
		Description: "由 Meta AI 研发并开源，在编码、推理及知识应用等场景表现优秀，暂不支持中文输出",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/llama2.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelLlama2_13b,
		Name:        "Llama 2 13B 英文版",
		ShortName:   "Llama2 13B",
		Description: "由 Meta AI 研发并开源，在编码、推理及知识应用等场景表现优秀，暂不支持中文输出",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/llama2.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelLlama2_7b_CN,
		Name:        "Llama 2 7B 中文版",
		ShortName:   "Llama2 7B",
		Description: "由 Meta AI 研发并开源，在编码、推理及知识应用等场景表现优秀，当前版本是千帆团队的中文增强版本",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/llama2-cn.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelChatGLM2_6B_32K,
		Name:        "ChatGLM2 6B",
		ShortName:   "ChatGLM2",
		Description: "ChatGLM2-6B 是由智谱 AI 与清华 KEG 实验室发布的中英双语对话模型，具备强大的推理性能、效果、较低的部署门槛及更长的上下文",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/chatglm.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelAquilaChat7B,
		Name:        "AquilaChat 7B",
		ShortName:   "AquilaChat",
		Description: "AquilaChat-7B 是由智源研究院研发，支持流畅的文本对话及多种语言类生成任务，通过定义可扩展的特殊指令规范",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/aquila.png",
	})
	models = append(models, Model{
		ID:          "文心千帆:" + baidu.ModelBloomz7B,
		Name:        "BLOOMZ 7B",
		ShortName:   "BLOOMZ",
		Description: "BLOOMZ-7B 是业内知名的⼤语⾔模型，由 BigScience 研发并开源，能够以46种语⾔和13种编程语⾔输出⽂本",
		Category:    "文心千帆",
		IsChat:      true,
		Disabled:    !conf.EnableBaiduWXAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/BLOOMZ.png",
	})

	if conf.EnableDashScopeAI {
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelQWenTurbo,
			Name:        "通义千问 Turbo",
			ShortName:   "千问 Turbo",
			Description: "通义千问超大规模语言模型，支持中文英文等不同语言输入",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.3",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-turbo.jpeg",
		})
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelQWenPlus,
			Name:        "通义千问 Plus",
			ShortName:   "千问 Plus",
			Description: "通义千问超大规模语言模型增强版，支持中文英文等不同语言输入",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.3",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-plus.jpeg",
		})
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelQWenMax,
			Name:        "通义千问 Max",
			ShortName:   "千问 Max",
			Description: "通义千问超大规模语言模型增强版，支持中文英文等不同语言输入",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.3",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-max.jpeg",
		})
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelQWenMaxLongContext,
			Name:        "通义千问 Max+",
			ShortName:   "千问 Max+",
			Description: "通义千问 Max Long Context 版本，支持 30K 上下文",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.3",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-max-longcontext.jpeg",
		})
		models = append(models, Model{
			ID:            "灵积:" + dashscope.ModelQWenVLPlus,
			Name:          "通义千问（视觉）",
			ShortName:     "千问 VL",
			Description:   "通义千问 VL 具备通用 OCR、视觉推理、中文文本理解基础能力，还能处理各种分辨率和规格的图像，甚至能“看图做题”",
			Category:      "灵积",
			IsChat:        true,
			Disabled:      !conf.EnableDashScopeAI,
			VersionMin:    "1.0.8",
			SupportVision: true,
			AvatarURL:     "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-vlplus.jpeg",
		})
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelQWen7BChat,
			Name:        "通义千问 7B",
			ShortName:   "千问 7B",
			Description: "通义千问 7B 是阿里云研发的通义千问大模型系列的 70 亿参数规模的模型，开源",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.3",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-osc-2.jpeg",
		})
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelQWen14BChat,
			Name:        "通义千问 14B",
			ShortName:   "千问 14B",
			Description: "通义千问 14B 是阿里云研发的通义千问大模型系列的 140 亿参数规模的模型，开源",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.3",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/qwen-osc-1.jpeg",
		})
		models = append(models, Model{
			ID:          "灵积:" + dashscope.ModelBaiChuan7BChatV1,
			Name:        "百川2 7B",
			ShortName:   "百川2 7B",
			Description: "由百川智能研发的大语言模型，融合了意图理解、信息检索以及强化学习技术，结合有监督微调与人类意图对齐，在知识问答、文本创作领域表现突出",
			Category:    "灵积",
			IsChat:      true,
			Disabled:    !conf.EnableDashScopeAI,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/baichuan.jpg",
		})
	}

	models = append(models, Model{
		ID:          "商汤日日新:" + string(sensenova.ModelNovaPtcXLV1),
		Name:        "商汤日日新（大）",
		ShortName:   "日日新（大）",
		Description: "商汤科技自主研发的超大规模语言模型，能够回答问题、创作文字，还能表达观点、撰写代码",
		Category:    "商汤日日新",
		IsChat:      true,
		Disabled:    !conf.EnableSenseNovaAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/sensenova.png",
	})
	models = append(models, Model{
		ID:          "商汤日日新:" + string(sensenova.ModelNovaPtcXSV1),
		Name:        "商汤日日新（小）",
		ShortName:   "日日新（小）",
		Description: "商汤科技自主研发的超大规模语言模型，能够回答问题、创作文字，还能表达观点、撰写代码",
		Category:    "商汤日日新",
		IsChat:      true,
		Disabled:    !conf.EnableSenseNovaAI,
		VersionMin:  "1.0.3",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/sensenova.png",
	})

	models = append(models, Model{
		ID:          "腾讯:" + tencentai.ModelHyllm,
		Name:        "混元大模型",
		ShortName:   "混元",
		Description: "由腾讯研发的大语言模型，具备强大的中文创作能力，复杂语境下的逻辑推理能力，以及可靠的任务执行能力",
		Category:    "腾讯",
		IsChat:      true,
		Disabled:    !conf.EnableTencentAI,
		VersionMin:  "1.0.5",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/hunyuan.png",
	})

	models = append(models, Model{
		ID:          "百川:" + baichuan.ModelBaichuan2_53B,
		Name:        "百川2 53B",
		ShortName:   "百川2 53B",
		Description: "由百川智能研发的大语言模型，融合了意图理解、信息检索以及强化学习技术，结合有监督微调与人类意图对齐，在知识问答、文本创作领域表现突出",
		Category:    "百川",
		IsChat:      true,
		Disabled:    !conf.EnableBaichuan,
		VersionMin:  "1.0.5",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/baichuan.jpg",
	})

	models = append(models, Model{
		ID:          "360智脑:" + gpt360.Model360GPT_S2_V9,
		Name:        "360智脑",
		Description: "由 360 研发的大语言模型，拥有独特的语言理解能力，通过实时对话，解答疑惑、探索灵感，用AI技术帮人类打开智慧的大门",
		Category:    "360",
		IsChat:      true,
		Disabled:    !conf.EnableGPT360,
		VersionMin:  "1.0.5",
		AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gpt360.jpg",
	})

	if conf.EnableOneAPI {
		models = append(models, Model{
			ID:          "oneapi:chatglm_turbo",
			Name:        "ChatGLM Turbo",
			ShortName:   "ChatGLM Turbo",
			Description: "智谱 AI 发布的对话模型，具备强大的推理性能、效果、较低的部署门槛及更长的上下文",
			Category:    "oneapi",
			IsChat:      true,
			Disabled:    !str.In("chatglm_turbo", conf.OneAPISupportModels),
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/chatglm.png",
		})
		models = append(models, Model{
			ID:          "oneapi:chatglm_pro",
			Name:        "ChatGLM Pro",
			ShortName:   "ChatGLM Pro",
			Description: "智谱 AI 发布的对话模型，具备强大的推理性能、效果、较低的部署门槛及更长的上下文",
			Category:    "oneapi",
			IsChat:      true,
			Disabled:    !str.In("chatglm_pro", conf.OneAPISupportModels),
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/chatglm.png",
		})
		models = append(models, Model{
			ID:          "oneapi:chatglm_std",
			Name:        "ChatGLM Std",
			ShortName:   "ChatGLM Std",
			Description: "智谱 AI 发布的对话模型，具备强大的推理性能、效果、较低的部署门槛及更长的上下文",
			Category:    "oneapi",
			IsChat:      true,
			Disabled:    !str.In("chatglm_std", conf.OneAPISupportModels),
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/chatglm.png",
		})
		models = append(models, Model{
			ID:          "oneapi:chatglm_lite",
			Name:        "ChatGLM Lite",
			ShortName:   "ChatGLM Lite",
			Description: "智谱 AI 发布的对话模型，具备强大的推理性能、效果、较低的部署门槛及更长的上下文",
			Category:    "oneapi",
			IsChat:      true,
			Disabled:    !str.In("chatglm_lite", conf.OneAPISupportModels),
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/chatglm.png",
		})
		models = append(models, Model{
			ID:          "oneapi:PaLM-2",
			Name:        "Google PaLM-2（英文版）",
			ShortName:   "PaLM-2",
			Description: "PaLM 2 是谷歌的下一代大规模语言模型",
			Category:    "oneapi",
			IsChat:      true,
			Disabled:    !str.In("PaLM-2", conf.OneAPISupportModels),
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/google-palm2.png",
		})
	}

	if conf.EnableOpenRouter {
		models = append(models, Model{
			ID:          "openrouter:01-ai.yi-34b-chat",
			Name:        "零一万物 Yi 34B",
			ShortName:   "Yi",
			Description: "零一万物打造的开源大语言模型，在多项评测中全球领跑，MMLU 等评测取得了多项 SOTA 国际最佳性能指标表现，以更小模型尺寸评测超越 LLaMA2-70B、Falcon-180B 等大尺寸开源模型",
			Category:    "openrouter",
			IsChat:      true,
			Disabled:    !str.In("01-ai/yi-34b-chat", conf.OpenRouterSupportModels),
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/yi-01.png",
		})
	}

	if conf.EnableSky {
		models = append(models, Model{
			ID:          "sky:SkyChat-MegaVerse",
			Name:        "天工 MegaVerse",
			ShortName:   "天工",
			Description: "昆仑万维研发的大语言模型，具备强大的中文创作能力，复杂语境下的逻辑推理能力，以及可靠的任务执行能力",
			Category:    "sky",
			IsChat:      true,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/sky.png",
		})
	}

	return models
}

func googleModels(conf *config.Config) []Model {
	return []Model{
		{
			ID:          "google:" + google.ModelGeminiPro,
			Name:        "Google Gemini Pro",
			Description: "Google 最新推出的 Gemini Pro 大语言模型",
			Category:    "google",
			IsChat:      true,
			Disabled:    !conf.EnableGoogleAI,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/gemini.png",
		},
		{
			ID:            "google:" + google.ModelGeminiProVision,
			Name:          "Google Gemini Pro（视觉）",
			Description:   "Google 最新推出的 Gemini Pro 大语言模型，该版本为视觉版，支持图片输入，但是不支持多轮对话",
			Category:      "google",
			IsChat:        true,
			SupportVision: true,
			Disabled:      !conf.EnableGoogleAI,
			VersionMin:    "1.0.8",
			AvatarURL:     "https://ssl.aicode.cc/ai-server/assets/avatar/gemini.png",
		},
	}
}

func anthropicModels(conf *config.Config) []Model {
	return []Model{
		{
			ID:          "Anthropic:" + string(anthropic.ModelClaudeInstant),
			Name:        "Claude instant",
			ShortName:   "Claude",
			Description: "Anthropic's fastest model, with strength in creative tasks. Features a context window of 9k tokens (around 7,000 words).",
			Category:    "Anthropic",
			IsChat:      true,
			Disabled:    !conf.EnableAnthropic,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/anthropic-claude-instant.png",
		},
		{
			ID:          "Anthropic:" + string(anthropic.ModelClaude2),
			Name:        "Claude 2.1",
			ShortName:   "Claude2",
			Description: "Anthropic's most powerful model. Particularly good at creative writing.",
			Category:    "Anthropic",
			IsChat:      true,
			Disabled:    !conf.EnableAnthropic,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/anthropic-claude-2.png",
		},
	}
}

func aideaModels(conf *config.Config) []Model {
	return []Model{
		{
			ID:          "virtual:nanxian",
			Name:        "南贤 3.5",
			ShortName:   "南贤 3.5",
			Description: "速度快，成本低",
			Category:    "virtual",
			IsChat:      true,
			Disabled:    !conf.EnableVirtualModel,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/nanxian.png",
		},
		{
			ID:          "virtual:beichou",
			Name:        "北丑 4.0",
			ShortName:   "北丑 4.0",
			Description: "能力强，更精准",
			Category:    "virtual",
			IsChat:      true,
			Disabled:    !conf.EnableVirtualModel,
			VersionMin:  "1.0.5",
			AvatarURL:   "https://ssl.aicode.cc/ai-server/assets/avatar/nanxian.png",
		},
	}
}
