package okx

import (
 "context"
 "net/url"
 "os"
 "testing"
 "time"
)

func TestLiveOKXSwapMetadata(t *testing.T) {
 if os.Getenv("QNEXT_OKX_SWAP_SMOKE")!="1" {t.Skip("enable swap external test")}
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second)
 defer cancel()
 var result apiResponse[struct {
  InstID string `json:"instId"`
  InstType string `json:"instType"`
  State string `json:"state"`
  CtVal string `json:"ctVal"`
  CtValCcy string `json:"ctValCcy"`
  CtType string `json:"ctType"`
  SettleCcy string `json:"settleCcy"`
  Uly string `json:"uly"`
 }]
 if err:=(Client{}).get(ctx,"/api/v5/public/instruments",url.Values{"instType":{"SWAP"},"instId":{"XAU-USDT-SWAP"}},&result);err!=nil{t.Fatal(err)}
 t.Logf("OKX SWAP code=%s msg=%s entries=%d",result.Code,result.Msg,len(result.Data))
 for _,s:=range result.Data{t.Logf("SWAP instId=%s instType=%s state=%s ctVal=%s ctValCcy=%s ctType=%s settleCcy=%s uly=%s",s.InstID,s.InstType,s.State,s.CtVal,s.CtValCcy,s.CtType,s.SettleCcy,s.Uly)}
 if result.Code!="0"||len(result.Data)!=1||result.Data[0].InstID!="XAU-USDT-SWAP"{t.Fatal("exact active XAU-USDT-SWAP metadata unavailable")}
}
