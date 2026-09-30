package service

func tts31SystemVoices() []TTSSystemVoice {
	items := []TTSSystemVoice{}
	groups := []struct {
		Languages []string
		Voices    [][2]string
	}{
		{Languages: []string{"zh", "ja", "ko", "fr", "de", "pt", "it", "vi", "id"}, Voices: [][2]string{
			{"longanhuan_v3.1", "龙安欢"}, {"longanlingxin_v3.1", "龙安灵心"}, {"longanfengyue_v3.1", "龙安风悦"}, {"xunanchuan_v3.1", "许南川"},
		}},
		{Languages: []string{"zh"}, Voices: [][2]string{
			{"yuxiaoyun_v3.1", "于小云"}, {"qiaoxiaojiao_v3.1", "乔小娇"}, {"xiaxiaochen_v3.1", "夏小晨"}, {"anmingyuan_v3.1", "安明远"},
			{"wenhuaiqing_v3.1", "温怀清"}, {"anxiaolan_v3.1", "安小岚"}, {"xieshurou_v3.1", "谢舒柔"}, {"baiqinglan_v3.1", "白清岚"},
			{"xuyuyuan_v3.1", "许玉远"}, {"anruorou_v3.1", "安若柔"}, {"wenhuaizhi_v3.1", "闻怀之"}, {"xiaoxingzhi_v3.1", "萧行之"},
			{"guyunshu_v3.1", "顾云舒"}, {"huozhuoshi_v3.1", "霍拙石"}, {"yeqinghe_v3.1", "叶清禾"}, {"yunhuanhuan_v3.1", "云欢欢"},
			{"xuxiaoqiao_v3.1", "徐小俏"}, {"baianran_v3.1", "白安然"}, {"xuyanchu_v3.1", "许言初"}, {"yezhiqing_v3.1", "叶知晴"},
			{"andi_v3.1", "安迪"}, {"anyuqing_v3.1", "安语晴"},
		}},
		{Languages: []string{"en"}, Voices: [][2]string{
			{"Emily_v3.1", "Emily"}, {"Luna_v3.1", "Luna"}, {"Eric_v3.1", "Eric"}, {"Luca_v3.1", "Luca"}, {"Abby_v3.1", "Abby"},
			{"Annie_v3.1", "Annie"}, {"Ava_v3.1", "Ava"}, {"Beth_v3.1", "Beth"}, {"Betty_v3.1", "Betty"}, {"Cally_v3.1", "Cally"},
			{"Cindy_v3.1", "Cindy"}, {"Donna_v3.1", "Donna"}, {"Andy_v3.1", "Andy"}, {"Brian_v3.1", "Brian"}, {"David_v3.1", "David"},
		}},
		{Languages: []string{"zh"}, Voices: [][2]string{
			{"longanyuanfei_v3.1", "龙安元妃"}, {"longjielidou_v3.1", "龙杰力豆"}, {"longanlingxi_v3.1", "龙安灵希"}, {"longhuohuo_v3.1", "龙火火"},
			{"longyingtao_v3.1", "龙应桃"}, {"longanya_v3.1", "龙安雅"}, {"longwan_v3.1", "龙婉"}, {"longxing_v3.1", "龙星"},
			{"longhua_v3.1", "龙华"}, {"longhan_v3.1", "龙寒"}, {"longanzhi_v3.1", "龙安智"}, {"longzhe_v3.1", "龙哲"},
			{"longanyang_v3.1", "龙安洋"}, {"libai_v3.1", "李白"}, {"longling_v3.1", "龙铃"}, {"longniuniu_v3.1", "龙牛牛"},
			{"longshanshan_v3.1", "龙闪闪"}, {"longpaopao_v3.1", "龙泡泡"}, {"loongstella_v3.1", "loongstella"}, {"longyuan_v3.1", "龙媛"},
			{"longmiao_v3.1", "龙妙"}, {"longsanshu_v3.1", "龙三叔"}, {"longanli_v3.1", "龙安莉"}, {"longanwen_v3.1", "龙安温"},
			{"longanlang_v3.1", "龙安朗"}, {"longxiaoxia_v3.1", "龙小夏"}, {"longanchong_v3.1", "龙安冲"},
		}},
	}
	for _, group := range groups {
		for _, voice := range group.Voices {
			items = append(items, TTSSystemVoice{ID: voice[0], Name: voice[1], TargetModel: "qwen-audio-3.1-tts-flash", Languages: append([]string{}, group.Languages...), Kind: "system"})
		}
	}
	return items
}
