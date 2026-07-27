package cloud189

import "litepan/pkg/jsonvalue"

type flexString = jsonvalue.FlexibleString

type Addition struct {
	RefreshToken string     `json:"refresh_token" label:"Token" type:"password" form:"required,full"`
	DeleteMode   string     `json:"delete_mode" label:"删除模式" type:"select" options:"trash:移动到回收站,delete:永久删除" default:"trash" form:"pair=opts1"`
	DownloadMode string     `json:"download_mode" label:"下载模式" type:"select" options:"redirect:302重定向,proxy:本机代理" default:"redirect" form:"pair=opts1"`
	RootFolderID string     `json:"root_folder_id" label:"根目录ID（默认 -11）" default:"-11" form:"pair=opts2"`
	CacheTTL     flexString `json:"cache_ttl" label:"缓存时间(分钟)" type:"number" default:"30" form:"pair=opts2"`
}
