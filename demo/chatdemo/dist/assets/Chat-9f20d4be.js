var xe=Object.defineProperty;var ye=(h,s,o)=>s in h?xe(h,s,{enumerable:!0,configurable:!0,writable:!0,value:o}):h[s]=o;var V=(h,s,o)=>(ye(h,typeof s!="symbol"?s+"":s,o),o);import{d as le,r as _,o as ce,W as r,a as re,b as y,c as S,F as K,e as ie,C as ue,f as Se,g as B,h as F,n as Ie,i as H,t as T,j as e,u as Y,k as J,l as N,A as R,_ as he,m as we,p as ee,q as te,w as ne,v as se,s as De,x as ke,T as Te,M as G,y as ae,S as $e,z,B as O,D as Le,E as be,G as Ee,P as oe,H as qe}from"./index-aeaf23aa.js";class j{constructor(s){V(this,"conversation");V(this,"avatarHashTag");this.conversation=s}get channel(){return this.conversation.channel}get channelInfo(){return this.conversation.channelInfo}get unread(){return this.conversation.unread}get timestamp(){return this.conversation.timestamp}set timestamp(s){this.conversation.timestamp=s}get timestampString(){return Ne(this.timestamp*1e3,!0)}get lastMessage(){return this.conversation.lastMessage}set lastMessage(s){this.conversation.lastMessage=s}get isMentionMe(){return this.conversation.isMentionMe}get remoteExtra(){return this.conversation.remoteExtra}set isMentionMe(s){this.conversation.isMentionMe=s}get reminders(){return this.conversation.reminders}get simpleReminders(){return this.conversation.simpleReminders}reloadIsMentionMe(){return this.conversation.reloadIsMentionMe()}get extra(){return this.conversation.extra||(this.conversation.extra={}),this.conversation.extra}isEqual(s){return this.conversation.isEqual(s.conversation)}}function Ne(h,s){var o=new Date,d=new Date(h),D=o.getFullYear(),I=o.getMonth()+1,L=o.getDate(),k=d.getFullYear(),f=d.getMonth()+1,v=d.getDate(),p="",m=s?" "+A(d,"hh:mm"):"";if(D===k){var $=o.getTime(),l=h,i=$-l;if(I===f&&L===v)i<60*1e3?p="刚刚":p=A(d,"hh:mm");else{var a=new Date;a.setDate(a.getDate()-1);var g=new Date;if(g.setDate(g.getDate()-2),f===a.getMonth()+1&&v===a.getDate())p="昨天"+m;else if(f===g.getMonth()+1&&v===g.getDate())p="前天"+m;else{var x=i/36e5;if(x<=7*24){var u=new Array(7);u[0]="星期日",u[1]="星期一",u[2]="星期二",u[3]="星期三",u[4]="星期四",u[5]="星期五",u[6]="星期六";var M=u[d.getDay()];p=M+m}else p=A(d,"yyyy/M/d")+m}}}else p=A(d,"yyyy/M/d")+m;return p}var A=function(h,s){var o={"M+":h.getMonth()+1,"d+":h.getDate(),"h+":h.getHours(),"m+":h.getMinutes(),"s+":h.getSeconds(),"q+":Math.floor((h.getMonth()+3)/3),S:h.getMilliseconds()};/(y+)/.test(s)&&(s=s.replace(RegExp.$1,(h.getFullYear()+"").substr(4-RegExp.$1.length)));for(var d in o)new RegExp("("+d+")").test(s)&&(s=s.replace(RegExp.$1,RegExp.$1.length===1?o[d]:("00"+o[d]).substr((""+o[d]).length)));return s};const Re={class:"conversations"},Ue=["onClick"],Fe={class:"item-content"},Ae={class:"left"},Ke={key:0,class:"avatar",style:{width:"48px",height:"48px"}},Be=["src"],He={key:1,class:"avatar",style:{width:"48px",height:"48px"}},Pe={class:"right"},Ve={class:"right-item1"},Ge={class:"title"},ze={class:"time"},Oe={class:"right-item2"},je={class:"last-msg"},Ye={key:0,className:"reddot"},Je=le({__name:"index",props:{onSelectChannel:{type:Function}},setup(h){const s=h,o=_(),d=_(),D=async l=>{if(console.log("connectStatusListener",l),l===ue.Connected){const i=await r.shared().conversationManager.sync();i&&i.length>0&&(o.value=v(i.map(a=>new j(a))))}},I=l=>{console.log("收到CMD：",l);const i=l.content;if(i.cmd===Se.CMDTypeClearUnread){const a=new B(i.param.channelID,i.param.channelType);f(a)}},L=(l,i)=>{var a,g,x;if(i===F.add)o.value=[new j(l),...o.value||[]];else if(i===F.update){const u=(a=o.value)==null?void 0:a.findIndex(M=>M.channel.channelID===l.channel.channelID&&M.channel.channelType===l.channel.channelType);u!==void 0&&u>=0&&(o.value[u]=new j(l),o.value=v())}else if(i===F.remove){const u=(g=o.value)==null?void 0:g.findIndex(M=>M.channel.channelID===l.channel.channelID&&M.channel.channelType===l.channel.channelType);u&&u>=0&&((x=o.value)==null||x.splice(u,1))}},k=l=>{o.value=[...o.value||[]]},f=l=>{const i=r.shared().conversationManager.findConversation(l);i&&(i.unread=0,r.shared().conversationManager.notifyConversationListeners(i,F.update))};ce(async()=>{r.shared().connectManager.addConnectStatusListener(D),r.shared().conversationManager.addConversationListener(L),r.shared().chatManager.addCMDListener(I),r.shared().channelManager.addListener(k)}),re(()=>{r.shared().conversationManager.removeConversationListener(L),r.shared().connectManager.removeConnectStatusListener(D),r.shared().chatManager.removeCMDListener(I),r.shared().channelManager.removeListener(k)});const v=l=>{let i=l;return i||(i=o.value),!i||i.length<=0?[]:i.sort((g,x)=>{var b,U;let u=g.timestamp,M=x.timestamp;return((b=g.extra)==null?void 0:b.top)===1&&(u+=1e12),((U=x.extra)==null?void 0:U.top)===1&&(M+=1e12),M-u})},p=l=>{d.value=l,s&&s.onSelectChannel(l),R.shared.clearUnread(l),f(l)},m=l=>d.value&&d.value.isEqual(l.channel)?"conversation-item selected":"conversation-item",$=l=>{r.shared().channelManager.getChannelInfo(l)||r.shared().channelManager.fetchChannelInfo(l)};return(l,i)=>(y(),S("div",Re,[(y(!0),S(K,null,ie(o.value,a=>{var g,x,u;return y(),S("div",{class:Ie(m(a)),onClick:()=>{p(a.channel)}},[H(T($(a.channel))+" ",1),e("div",Fe,[e("div",Ae,[a.channel.channelType===Y(J)?(y(),S("div",Ke,[e("img",{src:(g=a.channelInfo)==null?void 0:g.logo,style:{width:"48px",height:"48px"}},null,8,Be)])):(y(),S("div",He,T((x=a.channelInfo)==null?void 0:x.title),1))]),e("div",Pe,[e("div",Ve,[e("div",Ge,T(a.channel.channelID),1),e("div",ze,T(a.timestampString),1)]),e("div",Oe,[e("div",je,T((u=a.lastMessage)==null?void 0:u.content.conversationDigest),1),a.unread>0?(y(),S("div",Ye,T(a.unread),1)):N("",!0)])])])],10,Ue)}),256))]))}});const Qe=he(Je,[["__scopeId","data-v-eb93adcf"]]),de=h=>(Le("data-v-dcff2a4d"),h=h(),be(),h),Xe={class:"chat"},Ze={class:"header"},We=de(()=>e("button",null,"聊天列表",-1)),et={class:"center"},tt={class:"right",style:{display:"flex","align-items":"center"}},nt=de(()=>e("a",{style:{"margin-right":"40px",display:"flex","align-items":"center","font-size":"12px"},href:"https://github.com/WuKongIM/WuKongIM","aria-label":"github",target:"_blank",rel:"noopener","data-v-7bc22406":"","data-v-36371990":""},[e("svg",{role:"img",width:"32px",viewBox:"0 0 24 24",xmlns:"http://www.w3.org/2000/svg"},[e("title",null,"GitHub"),e("path",{d:"M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"})]),H("    吴彦祖，点个Star呗 ")],-1)),st={class:"content"},at={class:"conversation-box"},ot={class:"message-box"},lt=["id"],ct={key:0,class:"status"},rt={class:"bubble right"},it={class:"text"},ut={class:"avatar"},ht=["src"],dt=["id"],vt={class:"avatar"},gt=["src"],ft={class:"bubble"},pt={class:"text"},_t={class:"footer"},mt=["placeholder","onKeydown"],Mt={class:"switch"},Ct={class:"item"},xt=["onClick","checked"],yt={class:"item"},St=["onClick","checked"],It=["placeholder"],wt=le({__name:"Chat",setup(h){const s=we(),o=_(null),d=_(!1),D=_(""),I=_("");let L=0;const k=_(""),f=_(!0),v=_(new B("",0)),p=_("请输入对方登录名"),m=_(!1),$=_(!1);_(!1);const l=_("请输入消息"),i=_(),a=_(new Array),g=s.currentRoute.value.query.uid||void 0,x=s.currentRoute.value.query.token||"token111";D.value=`${g||""}(未连接)`;let u,M,b;ce(()=>{(!R.shared.config.apiURL||R.shared.config.apiURL==="")&&(r.shared().connectManager.disconnect(),s.push({path:"/"})),R.shared.get("/route",{param:{uid:s.currentRoute.value.query.uid}}).then(t=>{console.log(t);let c=t.wss_addr;(!c||c==="")&&(c=t.ws_addr),U(c)}).catch(t=>{console.log(t)})});const U=t=>{const c=r.shared().config;g&&x&&(c.uid=g,c.token=x),c.addr=t,c.sendCountOfEach=1e5,r.shared().config=c,u=(n,C,w)=>{n==ue.Connected?w?D.value=`${g||""}(连接成功-节点:${w.nodeId})`:D.value=`${g||""}(连接成功)`:D.value=`${g||""}(断开)`},r.shared().connectManager.addConnectStatusListener(u),M=n=>{if(v.value.isEqual(n.channel)){if(n.streamOn){let C=!1;for(const w of a.value)if(w.streamNo===n.streamNo){let E=w.streams;const q=new Ee;q.clientMsgNo=n.clientMsgNo,q.streamSeq=n.streamSeq||0,q.content=n.content,E&&E.length>0?E.push(q):E=[q],w.streams=E,C=!0;break}C||a.value.push(n)}else a.value.push(n);P()}},r.shared().chatManager.addMessageListener(M),b=n=>{console.log(n),a.value.forEach(C=>{if(C.clientSeq==n.clientSeq){C.status=n.reasonCode==1?G.Normal:G.Fail;return}})},r.shared().chatManager.addMessageStatusListener(b),r.shared().connect()};re(()=>{r.shared().connectManager.removeConnectStatusListener(u),r.shared().chatManager.removeMessageListener(M),r.shared().chatManager.removeMessageStatusListener(b),r.shared().disconnect()});const ve=t=>{f.value=t.target.checked,f.value?p.value="请输入对方登录名":p.value="请输入群组ID"},ge=t=>{f.value=!t.target.checked,f.value?p.value="请输入对方登录名":p.value="请输入群组ID"},P=()=>{const t=o.value;t&&ae(function(){t.scrollTop=t.scrollHeight})},Q=async()=>{m.value=!0,$.value=!1;const t=await r.shared().chatManager.syncMessages(v.value,{limit:15,startMessageSeq:0,endMessageSeq:0,pullMode:oe.Up});m.value=!1,t&&t.length>0&&t.forEach(c=>{a.value.push(c)}),P()},fe=async()=>{if(a.value.length==0)return;const t=a.value[0];if(t.messageSeq==1){$.value=!0;return}const c=15,n=await r.shared().chatManager.syncMessages(v.value,{limit:c,startMessageSeq:t.messageSeq-1,endMessageSeq:0,pullMode:oe.Down});n.length<c&&($.value=!0),n&&n.length>0&&n.reverse().forEach(C=>{a.value.unshift(C)}),ae(function(){const C=o.value,w=document.getElementById(t.clientMsgNo);w&&(C.scrollTop=w.offsetTop)})},X=()=>{d.value=!d.value},pe=()=>{f.value?v.value=new B(k.value,J):v.value=new B(k.value,ee),f.value||R.shared.joinChannel(v.value.channelID,v.value.channelType,r.shared().config.uid||""),r.shared().conversationManager.findConversation(v.value)||r.shared().conversationManager.createEmptyConversation(v.value),d.value=!1,a.value=[],Q()},_e=t=>{v.value=t,k.value=t.channelID,f.value=t.channelType==J,d.value=!1,a.value=[],Q()},Z=()=>{(!I.value||I.value.trim()==="")&&(L++,I.value=`${L}`);const t=$e.fromUint8(0);if(v.value&&v.value.channelID!=""){var c;i.value&&i.value!==""&&(t.streamNo=i.value),c=new z(I.value),r.shared().chatManager.send(c,v.value,t),I.value=""}else d.value=!0;P()},me=()=>{r.shared().connectManager.disconnect(),s.push({path:"/"})},W=t=>{if(t instanceof qe){const c=t.streams;let n="";if(t.content instanceof z&&(n=t.content.text||""),c&&c.length>0){for(const C of c)if(C.content instanceof z){const w=C.content;n=n+(w.text||"")}}return n}return"未知消息"},Me=t=>{const c=t.target.scrollTop;if(t.target.scrollHeight-(c+t.target.clientHeight),c<=250){if(m.value||$.value){console.log("不允许下拉","pulldowning",m.value,"pulldownFinished",$.value);return}console.log("下拉"),m.value=!0,fe().then(()=>{m.value=!1}).catch(()=>{m.value=!1})}},Ce=()=>{Z()};return(t,c)=>(y(),S(K,null,[e("div",Xe,[e("div",Ze,[e("div",{class:"left"},[e("button",{onClick:me},"退出"),We]),e("div",et,T(D.value),1),e("div",tt,[nt,e("button",{onClick:X},T(v.value.channelID.length==0?"与谁会话？":`${v.value.channelType==Y(ee)?"群":"单聊"}${v.value.channelID}`),1)])]),e("div",st,[e("div",at,[te(Qe,{onSelectChannel:_e})]),e("div",ot,[e("div",{class:"message-list",onScroll:Me,ref_key:"chatRef",ref:o},[(y(!0),S(K,null,ie(a.value,n=>(y(),S(K,null,[n.send?(y(),S("div",{key:0,class:"message right",id:n.clientMsgNo},[n.status!=Y(G).Normal?(y(),S("div",ct,"发送中")):N("",!0),e("div",rt,[e("div",it,T(W(n)),1)]),e("div",ut,[e("img",{src:`https://api.multiavatar.com/${n.fromUID}.png`,style:{width:"40px",height:"40px"}},null,8,ht)])],8,lt)):N("",!0),n.send?N("",!0):(y(),S("div",{key:1,class:"message",id:n.clientMsgNo},[e("div",vt,[e("img",{src:`https://api.multiavatar.com/${n.fromUID}.png`,style:{width:"40px",height:"40px"}},null,8,gt)]),e("div",ft,[e("div",pt,T(W(n)),1)])],8,dt))],64))),256))],544),e("div",_t,[ne(e("input",{placeholder:l.value,"onUpdate:modelValue":c[0]||(c[0]=n=>I.value=n),style:{height:"40px"},onKeydown:De(Ce,["enter"])},null,40,mt),[[se,I.value]]),e("button",{onClick:Z},"发送")])])])]),te(Te,{name:"fade"},{default:ke(()=>[d.value?(y(),S("div",{key:0,class:"setting",onClick:X},[e("div",{class:"setting-content",onClick:c[2]||(c[2]=O(()=>{},["stop"]))},[e("div",Mt,[e("div",Ct,[e("input",{type:"radio",onClick:O(ve,["stop"]),checked:f.value},null,8,xt),H("单聊 ")]),e("div",yt,[e("input",{type:"radio",onClick:O(ge,["stop"]),checked:!f.value},null,8,St),H("群聊 ")])]),ne(e("input",{placeholder:p.value,class:"to","onUpdate:modelValue":c[1]||(c[1]=n=>k.value=n)},null,8,It),[[se,k.value]]),e("button",{class:"ok",onClick:pe},"确定")])])):N("",!0)]),_:1})],64))}});const Tt=he(wt,[["__scopeId","data-v-dcff2a4d"]]);export{Tt as default};