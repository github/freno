/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package haproxy

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

var csv0 = `# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,
mysqlcluster0rw,FRONTEND,,,0,0,20000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,2,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
mysqlcluster0_rw_main,mysqlcluster0a-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032064,0,,1,3,1,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_rw_main,mysqlcluster0b-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032063,0,,1,3,2,,0,,2,0,,0,L7OKC,404,15,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_rw_main,mysqlcluster0c-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP,10,1,0,0,0,1032064,0,,1,3,3,,0,,2,0,,0,L7OK,200,17,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_rw_main,mysqlcluster0d-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032062,0,,1,3,4,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_rw_main,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,10,1,0,,0,1032064,0,,1,3,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
mysqlcluster0_rw_panic,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,4,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
mysqlcluster0ro,FRONTEND,,,0,0,20000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,5,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
mysqlcluster0_ro_main,mysqlcluster0a-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,DOWN,10,1,0,49,6,89174,368958,,1,6,1,,0,,2,0,,0,L7OK,200,18,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0b-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,DOWN,10,1,0,41,5,1912,1000,,1,6,2,,0,,2,0,,0,L7OK,200,24,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0c-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032061,0,,1,6,3,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0d-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032061,0,,1,6,4,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,20,2,0,,4,89174,728,,1,6,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
mysqlcluster0_ro_backup,mysqlcluster0e-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,DOWN,10,1,0,8,1,1028724,125,,1,7,1,,0,,2,0,,0,L7OK,200,16,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_backup,mysqlcluster0f-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,DOWN,10,1,0,8,1,1028738,96,,1,7,2,,0,,2,0,,0,L7OK,200,20,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_backup,mysqlcluster0g-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032059,0,,1,7,3,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_backup,mysqlcluster0h-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP,10,1,0,16,2,1028731,616,,1,7,4,,0,,2,0,,0,L7OK,200,16,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_backup,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,30,3,0,,1,1028738,96,,1,7,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
monitoring,FRONTEND,,,0,0,2000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,8,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
monitoring,BACKEND,0,0,0,0,200,0,0,0,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,8,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
statsctl,FRONTEND,,,1,3,2000,21718,2788357,173364223,0,0,315,,,,,OPEN,,,,,,,,,1,9,0,,,,0,1,0,3,,,,0,21403,0,315,0,0,,1,3,21719,,,0,0,0,0,,,,,,,,
statsctl,BACKEND,0,0,0,0,200,0,2788357,173364223,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,9,0,,0,,1,0,,0,,,,0,0,0,0,0,0,,,,,0,0,0,0,0,0,0,,,0,0,0,0,
`

var csvTransitioning = `# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,
mysqlcluster0ro,FRONTEND,,,0,0,20000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,5,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
mysqlcluster0_ro_main,mysqlcluster0a-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP 1/2,10,1,0,49,6,89174,368958,,1,6,1,,0,,2,0,,0,L7OK,200,18,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0b-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP,10,1,0,41,5,1912,1000,,1,6,2,,0,,2,0,,0,L7OK,200,24,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0c-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032061,0,,1,6,3,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0d-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032061,0,,1,6,4,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,20,2,0,,4,89174,728,,1,6,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
monitoring,FRONTEND,,,0,0,2000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,8,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
monitoring,BACKEND,0,0,0,0,200,0,0,0,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,8,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
statsctl,FRONTEND,,,1,3,2000,21718,2788357,173364223,0,0,315,,,,,OPEN,,,,,,,,,1,9,0,,,,0,1,0,3,,,,0,21403,0,315,0,0,,1,3,21719,,,0,0,0,0,,,,,,,,
statsctl,BACKEND,0,0,0,0,200,0,2788357,173364223,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,9,0,,0,,1,0,,0,,,,0,0,0,0,0,0,,,,,0,0,0,0,0,0,0,,,0,0,0,0,
`

var csvTransitioningAllUp = `# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,
mysqlcluster0ro,FRONTEND,,,0,0,20000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,5,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
mysqlcluster0_ro_main,mysqlcluster0a-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP 1/2,10,1,0,49,6,89174,368958,,1,6,1,,0,,2,0,,0,L7OK,200,18,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0b-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP 1/2,10,1,0,41,5,1912,1000,,1,6,2,,0,,2,0,,0,L7OK,200,24,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0c-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032061,0,,1,6,3,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0d-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB,10,1,0,0,0,1032061,0,,1,6,4,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,20,2,0,,4,89174,728,,1,6,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
monitoring,FRONTEND,,,0,0,2000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,8,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
monitoring,BACKEND,0,0,0,0,200,0,0,0,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,8,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
statsctl,FRONTEND,,,1,3,2000,21718,2788357,173364223,0,0,315,,,,,OPEN,,,,,,,,,1,9,0,,,,0,1,0,3,,,,0,21403,0,315,0,0,,1,3,21719,,,0,0,0,0,,,,,,,,
statsctl,BACKEND,0,0,0,0,200,0,2788357,173364223,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,9,0,,0,,1,0,,0,,,,0,0,0,0,0,0,,,,,0,0,0,0,0,0,0,,,0,0,0,0,
`

var csvTransitioningAll = `# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,
mysqlcluster0ro,FRONTEND,,,0,0,20000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,5,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
mysqlcluster0_ro_main,mysqlcluster0a-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP 1/2,10,1,0,49,6,89174,368958,,1,6,1,,0,,2,0,,0,L7OK,200,18,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0b-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP 1/2,10,1,0,41,5,1912,1000,,1,6,2,,0,,2,0,,0,L7OK,200,24,,,,,,,0,,,,0,0,,,,,-1,OK,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0c-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB 2/3,10,1,0,0,0,1032061,0,,1,6,3,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,mysqlcluster0d-dc,0,0,0,0,,0,0,0,,0,,0,0,0,0,NOLB 1/3,10,1,0,0,0,1032061,0,,1,6,4,,0,,2,0,,0,L7OKC,404,12,,,,,,,0,,,,0,0,,,,,-1,Not Found,,0,0,0,0,
mysqlcluster0_ro_main,BACKEND,0,0,0,0,2000,0,0,0,0,0,,0,0,0,0,UP,20,2,0,,4,89174,728,,1,6,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
monitoring,FRONTEND,,,0,0,2000,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,8,0,,,,0,0,0,0,,,,,,,,,,,0,0,0,,,0,0,0,0,,,,,,,,
monitoring,BACKEND,0,0,0,0,200,0,0,0,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,8,0,,0,,1,0,,0,,,,,,,,,,,,,,0,0,0,0,0,0,-1,,,0,0,0,0,
statsctl,FRONTEND,,,1,3,2000,21718,2788357,173364223,0,0,315,,,,,OPEN,,,,,,,,,1,9,0,,,,0,1,0,3,,,,0,21403,0,315,0,0,,1,3,21719,,,0,0,0,0,,,,,,,,
statsctl,BACKEND,0,0,0,0,200,0,2788357,173364223,0,0,,0,0,0,0,UP,0,0,0,,0,1032064,0,,1,9,0,,0,,1,0,,0,,,,0,0,0,0,0,0,,,,,0,0,0,0,0,0,0,,,0,0,0,0,
`

func init() {
	log.SetLevel(log.ERROR)
}

func TestToCsvUrl(t *testing.T) {
	{
		u, err := url.Parse("http://10.0.0.2:1234")
		test.S(t).ExpectNil(err)
		csvUrl := toCSVUrl(*u)
		test.S(t).ExpectEquals(csvUrl.String(), "http://10.0.0.2:1234/;csv;norefresh")
	}
	{
		u, err := url.Parse("http://10.0.0.2:1234/")
		test.S(t).ExpectNil(err)
		csvUrl := toCSVUrl(*u)
		test.S(t).ExpectEquals(csvUrl.String(), "http://10.0.0.2:1234/;csv;norefresh")
	}
	{
		u, err := url.Parse("http://10.0.0.2:1234/stats/pool")
		test.S(t).ExpectNil(err)
		csvUrl := toCSVUrl(*u)
		test.S(t).ExpectEquals(csvUrl.String(), "http://10.0.0.2:1234/stats/pool;csv;norefresh")
	}
}

func TestParseHeader(t *testing.T) {
	header := "# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,"
	tokensMap := parseHeader(header)
	test.S(t).ExpectEquals(tokensMap["pxname"], 0)
	test.S(t).ExpectEquals(tokensMap["svname"], 1)
	test.S(t).ExpectEquals(tokensMap["status"], 17)
	_, ok := tokensMap["no_such_element"]
	test.S(t).ExpectFalse(ok)
}

func TestParseHosts(t *testing.T) {
	{
		backendHosts, err := ParseCsvHosts(csv0, "mysqlcluster0_rw_main")
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(backendHosts), 4)

		hosts := FilterThrotllerHosts(backendHosts)
		test.S(t).ExpectTrue(reflect.DeepEqual(hosts, []string{"mysqlcluster0c-dc"}))
	}
	{
		backendHosts, err := ParseCsvHosts(csv0, "mysqlcluster0_ro_main")
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(backendHosts), 4)

		hosts := FilterThrotllerHosts(backendHosts)
		test.S(t).ExpectTrue(reflect.DeepEqual(hosts, []string{"mysqlcluster0a-dc", "mysqlcluster0b-dc"}))
	}
	{
		backendHosts, err := ParseCsvHosts(csv0, "mysqlcluster0_ro_backup")
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(backendHosts), 4)

		hosts := FilterThrotllerHosts(backendHosts)
		test.S(t).ExpectTrue(reflect.DeepEqual(hosts, []string{"mysqlcluster0e-dc", "mysqlcluster0f-dc", "mysqlcluster0h-dc"}))
	}
}

func TestParseHostsTransitioning(t *testing.T) {
	{
		backendHosts, err := ParseCsvHosts(csvTransitioning, "mysqlcluster0_ro_main")
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(backendHosts), 4)

		hosts := FilterThrotllerHosts(backendHosts)
		test.S(t).ExpectTrue(reflect.DeepEqual(hosts, []string{"mysqlcluster0b-dc"}))
	}
	{
		backendHosts, err := ParseCsvHosts(csvTransitioningAllUp, "mysqlcluster0_ro_main")
		test.S(t).ExpectEquals(err, HAProxyAllUpHostsTransitioning)
		test.S(t).ExpectEquals(len(backendHosts), 4)
	}
	{
		backendHosts, err := ParseCsvHosts(csvTransitioningAll, "mysqlcluster0_ro_main")
		test.S(t).ExpectEquals(err, HAProxyAllHostsTransitioning)
		test.S(t).ExpectEquals(len(backendHosts), 4)
	}
}

func TestParseStatus(t *testing.T) {
	{
		status, isTransitioning := ParseStatus("NOLB")
		test.S(t).ExpectFalse(isTransitioning)
		test.S(t).ExpectEquals(status, StatusNOLB)
	}
	{
		status, isTransitioning := ParseStatus("NOLB 1/2")
		test.S(t).ExpectTrue(isTransitioning)
		test.S(t).ExpectEquals(status, StatusNOLB)
	}
	{
		status, isTransitioning := ParseStatus("UP 1/2")
		test.S(t).ExpectTrue(isTransitioning)
		test.S(t).ExpectEquals(status, StatusUp)
	}
	{
		status, isTransitioning := ParseStatus("DOWN")
		test.S(t).ExpectFalse(isTransitioning)
		test.S(t).ExpectEquals(status, StatusDown)
	}
	{
		status, isTransitioning := ParseStatus("no check")
		test.S(t).ExpectFalse(isTransitioning)
		test.S(t).ExpectEquals(status, StatusNoCheck)
	}
}
