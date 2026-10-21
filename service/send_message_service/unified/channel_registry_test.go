package unified

import (
	"reflect"
	"testing"
)

func TestDtalkDingChannelRegistered(t *testing.T) {
	channel, ok := GetGlobalChannelRegistry().GetChannel("DtalkDing")
	if !ok {
		t.Fatal("DtalkDing channel is not registered")
	}

	if got := channel.GetSupportedFormats(); !reflect.DeepEqual(got, []string{"text"}) {
		t.Fatalf("DtalkDing supported formats = %#v, want [text]", got)
	}
}
