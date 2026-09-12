const {test}=require('node:test');
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function setup(name,rpc,engine={}) {
 const node=(tag,attrs={},children=[])=>{let value=attrs.value||'';return {tag,...attrs,children,disabled:false,
  get value(){return value||(tag==='select'?this.children[0]?.value||'':'');},set value(v){value=v;},
  get options(){return this.children;},appendChild(c){this.children.push(c);return c;},
  set innerHTML(_){this.children=[];},querySelectorAll(){return this.children.filter(n=>n.class==='card');}};};
 let section;const GUI={rpc,engine:async()=>engine,register(s){section=s;GUI.current=s;},el:node,
  card(title){const n=node('section',{class:'card',title});n.body=node('div');n.appendChild(n.body);return n;},
  stat:(k,v)=>node('stat',{text:k+': '+v}),table:(h,r)=>node('table',{},r.flat()),fmtHash:String,fmtHexNum:Number,fmtTKM:String,fmtTS:String,live:async()=>{},
  empty:text=>node('p',{text}),jsonView:obj=>node('pre',{text:JSON.stringify(obj)}),toast:()=>{},runAction:async(b,f)=>f()};
 vm.runInNewContext(fs.readFileSync(require.resolve('../web/js/'+name+'.js'),'utf8'),{window:{GUI,crypto:globalThis.crypto},GUI,TextEncoder,document:{addEventListener(){},hidden:false},location:{origin:'http://localhost'},localStorage:{getItem(){return null;},setItem(){}}});
 const view=node('div'),all=n=>[n,...(n.children||[]).flatMap(c=>typeof c==='object'?all(c):[])];
 return {section,view,find:(k,v)=>all(view).find(n=>n[k]===v),text:()=>all(view).map(n=>n.textContent||n.text||'').join(' ')};
}
test('Mail passes registration fields to the signed engine and clears password',async()=>{
 const owner='0x'+'1'.repeat(40),calls=[];
 const ui=setup('mail',async m=>m==='tkm_accountAlgorithms'?{[owner]:'ML-DSA-87'}:m==='emailvm_status'?{ready:true}:m==='tkm_exportPQAccount'?'encrypted':[],{
  keyfileFromHex:x=>x,async runMailOperation(o){calls.push(o);return ['0x123'];}});
 await ui.section.render(ui.view);
 ui.find('aria-label','Mail wallet password').value='test-only';ui.find('aria-label','Mailbox username').value='alice';
 await ui.find('text','Register mailbox').onclick();
 assert.equal(calls[0].operation,'buy');assert.equal(calls[0].params.domain,'tkm');assert.equal(calls[0].params.username,'alice');
 assert.equal(ui.find('aria-label','Mail wallet password').value,'');assert.match(ui.text(),/Confirmed/);
});
test('Mail pages through all inbox records',async()=>{
 const calls=[],owner='0x'+'1'.repeat(40);
 const ui=setup('mail',async(m,p)=>{calls.push([m,p]);if(m==='tkm_accountAlgorithms')return {[owner]:'ML-DSA-87'};if(m==='emailvm_status')return {ready:true};if(m==='emailvm_inboxPage')return {messages:[{id:p[1],from:'bob@tkm',timestamp:1}],nextOffset:'0x20',hasMore:p[1]==='0x0'};return [];});
 await ui.section.render(ui.view);ui.find('aria-label','Inbox mailbox').value='alice@tkm';
 await ui.find('text','Load messages').onclick();await ui.find('text','Load more').onclick();
 assert.equal(calls.filter(([m])=>m==='emailvm_inboxPage')[1][1][1],'0x20');assert.ok(ui.find('text','Decrypt message'));
});
test('Phone uses local PQ signing for registration and clears password',async()=>{
 const owner='0x'+'1'.repeat(40),pub='0x'+'ab'.repeat(2592),calls=[];
 const ui=setup('phone',async(m,p)=>{calls.push([m,p]);if(m==='tkm_accountAlgorithms')return {[owner]:'ML-DSA-87'};if(m==='tkmphone_signatureAlgorithms')return ['ML-DSA-87'];if(m==='tkmphone_status')return {active:true};if(m==='tkmphone_registeredNumber')return {number:{owner},devices:[]};if(m==='tkmphone_deviceKeySigningHash')return 'digest';if(m==='tkmphone_registerDeviceKey')return {active:true};return [];},{keyfileFromHex:()=>({publicKey:pub}),phonePublicKey:async()=>pub,signPhoneDigest:async(k,p,d)=>{assert.equal(d,'digest');return 'pq-signature';}});
 await ui.section.render(ui.view);
 for(const [label,value]of [['Phone number','123'],['Device name','Phone'],['Owner account',owner],['Wallet password','test-only']])ui.find('aria-label',label).value=value;
 await ui.find('text','Sign and register device').onclick();
 const call=calls.find(([m])=>m==='tkmphone_registerDeviceKey');assert.ok(call);assert.equal(call[1][2],pub);assert.equal(call[1][3],'pq-signature');
 assert.equal(ui.find('aria-label','Wallet password').value,'');assert.ok(!calls.some(([m])=>m==='tkm_signHashWithPassphrase'));
});
