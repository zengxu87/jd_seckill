package jd_seckill

import (
	"../common"
	"../conf"
	"../service"
	"errors"
	"fmt"
	"github.com/Albert-Zhan/httpc"
	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Seckill struct {
	client *httpc.HttpClient
	conf *conf.Config
	cookies *[]*http.Cookie
}

func NewSeckill(client *httpc.HttpClient,conf *conf.Config) *Seckill {
	return &Seckill{client: client,conf: conf}
}

func (this *Seckill) SetCookies(cookies *[]*http.Cookie) {
	this.cookies = cookies
}
func (this *Seckill) SkuTitle() (string,error) {
	skuId:=this.conf.Read("config","sku_id")
	req:=httpc.NewRequest(this.client)
	resp,body,err:=req.SetUrl(fmt.Sprintf("https://item.jd.com/%s.html",skuId)).SetMethod("get").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("访问商品详情失败")
		return "",errors.New("访问商品详情失败")
	}
	html:=strings.NewReader(body)
	doc,_:=goquery.NewDocumentFromReader(html)
	return strings.TrimSpace(doc.Find(".sku-name").Text()),nil
}
func (this *Seckill) waitForReserveTime(buyDate string) bool {
	nowLocalTime:=time.Now().UnixNano()/1e6
	loc, _ := time.LoadLocation("Local")
	t,_:=time.ParseInLocation("2006-01-02 15:04",buyDate,loc)
	buyTime:=t.UnixNano()/1e6

	log.Println(fmt.Sprintf("正在等待到达预订时间:%s",buyDate))
	timerTime:=buyTime - nowLocalTime
	if timerTime > 0 {
		time.Sleep(time.Duration(timerTime)*time.Millisecond)
	}
	log.Println(fmt.Sprintf("已到达预订时间:%s",buyDate))
	return true
}
func (this *Seckill) MakeReserve() (string, error ){
	var buyDate string
	shopTitle,err:=this.SkuTitle()
	if err!=nil {
		log.Println("获取商品信息失败")
		return buyDate, err
	}else{
		log.Println("商品名称:"+shopTitle)
	}
	skuId:=this.conf.Read("config","sku_id")
	req:=httpc.NewRequest(this.client)
	req.SetCookies(this.cookies)
	req.SetHeader("User-Agent",this.conf.Read("config","DEFAULT_USER_AGENT"))
	req.SetHeader("Host","yushou.jd.com")
	req.SetHeader("Connection","keep-alive")
	//req.SetHeader("Referer",fmt.Sprintf("https://item.jd.com/%s.html",skuId))
	resp,body,err:=req.SetUrl("https://item-soa.jd.com/getWareBusiness?skuId="+skuId).SetMethod("get").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("预约商品失败")
	}else{
		reserveUrl:=gjson.Get(body,"yuyueInfo.url").String()
		buyDate=gjson.Get(body,"yuyueInfo.buyTime").String()
		reserveDuration:=gjson.Get(body,"yuyueInfo.yuyueTime").String()
		sp := strings.Split(reserveDuration, "~")
		reserveTime := sp[0]
		log.Println("获取预约信息:",body)
		log.Println("reserveUrl:",reserveUrl)
		log.Println("buyDate:",buyDate)
		log.Println("reserveTime:",reserveTime)
		if this.waitForReserveTime(reserveTime) {
			req=httpc.NewRequest(this.client)
			req.SetCookies(this.cookies)
			req.SetHeader("Host","yushou.jd.com")
			req.SetHeader("Connection","keep-alive")
			req.SetHeader("Accept","text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
			req.SetHeader("Accept-Encoding","gzip, deflate, br")
			resp,body,err =req.SetUrl("https:"+reserveUrl).SetMethod("get").Send().End()
			if err == nil {
				log.Println("预约成功，已获得抢购资格 / 您已成功预约过了，无需重复预约")
				log.Println("resp=",resp)
				log.Println("body=",body)
				log.Printf("req=%++v",req)
			}
		}
	}
	return buyDate, err
}

func (this *Seckill) getSeckillUrl() (string,error) {
	skuId:=this.conf.Read("config","sku_id")
	req:=httpc.NewRequest(this.client)
	req.SetHeader("User-Agent",this.conf.Read("config","DEFAULT_USER_AGENT"))
	req.SetHeader("Host","itemko.jd.com")
	req.SetHeader("Referer",fmt.Sprintf("https://item.jd.com/%s.html",skuId))
	resp,body,err:=req.SetUrl("https://itemko.jd.com/itemShowBtn?callback=jQuery{}"+strconv.Itoa(common.Rand(1000000,9999999))+"&skuId="+skuId+"&from=pc&_="+strconv.Itoa(int(time.Now().Unix()*1000))).SetMethod("get").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("抢购链接获取失败，稍后自动重试")
		return "",errors.New("抢购链接获取失败，稍后自动重试")
	}
	url:=gjson.Get(body,"url").String()
	if url=="" {
		log.Println("抢购链接获取失败，稍后自动重试")
		return "",errors.New("抢购链接获取失败，稍后自动重试")
	}
	//https://divide.jd.com/user_routing?skuId=8654289&sn=c3f4ececd8461f0e4d7267e96a91e0e0&from=pc
	url=strings.ReplaceAll(url,"divide","marathon")
	//https://marathon.jd.com/captcha.html?skuId=8654289&sn=c3f4ececd8461f0e4d7267e96a91e0e0&from=pc
	url=strings.ReplaceAll(url,"user_routing","captcha.html")
	return url,nil
}

func (this *Seckill) RequestSeckillUrl()  {
	user:=NewUser(this.client,this.conf)
	userInfo,err:=user.GetUserInfo()
	if err!=nil {
		log.Println("获取用户信息失败")
	}else{
		log.Println("用户:"+userInfo)
	}
	shopTitle,err:=this.SkuTitle()
	if err!=nil {
		log.Println("获取商品信息失败")
	}else{
		log.Println("商品名称:"+shopTitle)
	}
	url,_:=this.getSeckillUrl()
	skuId:=this.conf.Read("config","sku_id")
	req:=httpc.NewRequest(this.client)
	req.SetHeader("User-Agent",this.conf.Read("config","DEFAULT_USER_AGENT"))
	req.SetHeader("Host","marathon.jd.com")
	req.SetHeader("Referer",fmt.Sprintf("https://item.jd.com/%s.html",skuId))
	_,_,_=req.SetUrl(url).SetMethod("get").Send().End()
}

func (this *Seckill) SeckillPage()  {
	log.Println("访问抢购订单结算页面...")
	skuId:=this.conf.Read("config","sku_id")
	seckillNum:=this.conf.Read("config","seckill_num")
	req:=httpc.NewRequest(this.client)
	req.SetHeader("User-Agent",this.conf.Read("config","DEFAULT_USER_AGENT"))
	req.SetHeader("Host","marathon.jd.com")
	req.SetHeader("Referer",fmt.Sprintf("https://item.jd.com/%s.html",skuId))
	_,_,_=req.SetUrl("https://marathon.jd.com/seckill/seckill.action?skuId="+skuId+"&num="+seckillNum+"&rid="+strconv.Itoa(int(time.Now().Unix()))).SetMethod("get").Send().End()
}

func (this *Seckill) SeckillInitInfo() (string,error) {
	log.Println("获取秒杀初始化信息...")
	skuId:=this.conf.Read("config","sku_id")
	seckillNum:=this.conf.Read("config","seckill_num")
	req:=httpc.NewRequest(this.client)
	req.SetHeader("User-Agent",this.conf.Read("config","DEFAULT_USER_AGENT"))
	req.SetHeader("Host","marathon.jd.com")
	req.SetParam("sku",skuId)
	req.SetParam("num",seckillNum)
	req.SetParam("isModifyAddress","false")
	resp,body,err:=req.SetUrl("https://marathon.jd.com/seckillnew/orderService/pc/init.action").SetMethod("post").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("初始化秒杀信息失败")
		return "",errors.New("初始化秒杀信息失败")
	}
	return body,nil
}

func (this *Seckill) SubmitSeckillOrder() bool {
	eid:=this.conf.Read("config","eid")
	fp:=this.conf.Read("config","fp")
	skuId:=this.conf.Read("config","sku_id")
	seckillNum:=this.conf.Read("config","seckill_num")
	paymentPwd:=this.conf.Read("account","payment_pwd")
	initInfo,_:=this.SeckillInitInfo()
	address:=gjson.Get(initInfo,"addressList").Array()
	defaultAddress:=address[0]
	isinvoiceInfo:=gjson.Get(initInfo,"invoiceInfo").Exists()
	invoiceTitle:="-1"
	invoiceContentType:="-1"
	invoicePhone:=""
	invoicePhoneKey:=""
	if isinvoiceInfo {
		invoiceTitle=gjson.Get(initInfo,"invoiceInfo.invoiceTitle").String()
		invoiceContentType=gjson.Get(initInfo,"invoiceInfo.invoiceContentType").String()
		invoicePhone=gjson.Get(initInfo,"invoiceInfo.invoicePhone").String()
		invoicePhoneKey=gjson.Get(initInfo,"invoiceInfo.invoicePhoneKey").String()
	}
	invoiceInfo:="false"
	if isinvoiceInfo {
		invoiceInfo="true"
	}
	token:=gjson.Get(initInfo,"token").String()
	log.Println("提交抢购订单...")
	req:=httpc.NewRequest(this.client)
	req.SetHeader("User-Agent",this.conf.Read("config","DEFAULT_USER_AGENT"))
	req.SetHeader("Host","marathon.jd.com")
	req.SetHeader("Referer",fmt.Sprintf("https://marathon.jd.com/seckill/seckill.action?skuId=%s&num=%s&rid=%d",skuId,seckillNum,int(time.Now().Unix())))
	req.SetParam("skuId",skuId)
	req.SetParam("num",seckillNum)
	req.SetParam("addressId",defaultAddress.Get("id").String())
	req.SetParam("yuShou","true")
	req.SetParam("isModifyAddress","false")
	req.SetParam("name",defaultAddress.Get("name").String())
	req.SetParam("provinceId",defaultAddress.Get("provinceId").String())
	req.SetParam("cityId",defaultAddress.Get("cityId").String())
	req.SetParam("countyId",defaultAddress.Get("countyId").String())
	req.SetParam("townId",defaultAddress.Get("townId").String())
	req.SetParam("addressDetail",defaultAddress.Get("addressDetail").String())
	req.SetParam("mobile",defaultAddress.Get("mobile").String())
	req.SetParam("mobileKey",defaultAddress.Get("mobileKey").String())
	req.SetParam("email",defaultAddress.Get("email").String())
	req.SetParam("postCode","")
	req.SetParam("invoiceTitle",invoiceTitle)
	req.SetParam("invoiceCompanyName","")
	req.SetParam("invoiceContent",invoiceContentType)
	req.SetParam("invoiceTaxpayerNO","")
	req.SetParam("invoiceEmail","")
	req.SetParam("invoicePhone",invoicePhone)
	req.SetParam("invoicePhoneKey",invoicePhoneKey)
	req.SetParam("invoice",invoiceInfo)
	req.SetParam("password",paymentPwd)
	req.SetParam("codTimeType","3")
	req.SetParam("paymentType","4")
	req.SetParam("areaCode","")
	req.SetParam("overseas","0")
	req.SetParam("phone","")
	req.SetParam("eid",eid)
	req.SetParam("fp",fp)
	req.SetParam("token",token)
	req.SetParam("pru","")
	resp,body,err:=req.SetUrl("https://marathon.jd.com/seckillnew/orderService/pc/submitOrder.action?skuId="+skuId).SetMethod("post").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("抢购失败，网络错误")
		if this.conf.Read("messenger","enable")=="true" && this.conf.Read("messenger","type")=="smtp" {
			email:=service.NerEmail(this.conf)
			_=email.SendMail([]string{this.conf.Read("messenger","email")},"茅台抢购通知","抢购失败，网络错误")
		}
		return false
	}
	if !gjson.Valid(body) {
		log.Println("抢购失败，返回信息:"+common.Substr(body,0,128))
		if this.conf.Read("messenger","enable")=="true" && this.conf.Read("messenger","type")=="smtp" {
			email:=service.NerEmail(this.conf)
			_=email.SendMail([]string{this.conf.Read("messenger","email")},"茅台抢购通知","抢购失败，返回信息:"+common.Substr(body,0,128))
		}
		return false
	}
	if gjson.Get(body,"success").Bool() {
		orderId:=gjson.Get(body,"orderId").String()
		totalMoney:=gjson.Get(body,"totalMoney").String()
		payUrl:="https:"+gjson.Get(body,"pcUrl").String()
		log.Println(fmt.Sprintf("抢购成功，订单号:%s, 总价:%s, 电脑端付款链接:%s",orderId,totalMoney,payUrl))
		if this.conf.Read("messenger","enable")=="true" && this.conf.Read("messenger","type")=="smtp" {
			email:=service.NerEmail(this.conf)
			_=email.SendMail([]string{this.conf.Read("messenger","email")},"茅台抢购通知",fmt.Sprintf("抢购成功，订单号:%s, 总价:%s, 电脑端付款链接:%s",orderId,totalMoney,payUrl))
		}
		return true
	}else{
		log.Println("抢购失败，返回信息:"+body)
		if this.conf.Read("messenger","enable")=="true" && this.conf.Read("messenger","type")=="smtp" {
			email:=service.NerEmail(this.conf)
			_=email.SendMail([]string{this.conf.Read("messenger","email")},"茅台抢购通知","抢购失败，返回信息:"+body)
		}
		return false
	}
}