import { useState } from "react";
import { Users, Globe, MessageSquare, FileText, MessageCircle, Trophy, Database, Settings, ChevronDown, Bell, Zap, Plus, Upload, Check, X, LogIn, RefreshCw, Send, Eye, ThumbsUp, Share2, Shield, Palette, CreditCard, UserPlus, Lock, Mail, ArrowLeft, BarChart2, Search } from "lucide-react";

// ── MOCK DATA ──────────────────────────────────────────────────────
const LEADS=[{id:1,name:"Nguyễn Văn An",status:"Hot",group:"Ô tô VN",agent:"Agent_01",last:"2g",score:92,phone:"0901 234 567"},{id:2,name:"Trần Thị Bích",status:"Warm",group:"Xe Miền Nam",agent:"Agent_02",last:"5g",score:75,phone:"0912 345 678"},{id:3,name:"Lê Hoàng Minh",status:"Cold",group:"Ô tô VN",agent:"Agent_01",last:"1 ngày",score:45,phone:"0923 456 789"},{id:4,name:"Phạm Thu Hà",status:"Hot",group:"Nội thất",agent:"Agent_03",last:"30p",score:88,phone:"0934 567 890"},{id:5,name:"Vũ Đình Tùng",status:"Warm",group:"Xe Miền Bắc",agent:"Agent_02",last:"3g",score:67,phone:"0945 678 901"},{id:6,name:"Đặng Ngọc Lan",status:"Hot",group:"Ô tô VN",agent:"Agent_01",last:"1g",score:95,phone:"0956 789 012"}];
const THREADS=[{id:1,lead:"Nguyễn Văn An",agent:"Agent_01",last:"Anh ơi xe còn hàng không ạ?",time:"2p",status:"Active",unread:2},{id:2,lead:"Phạm Thu Hà",agent:"Agent_03",last:"Em muốn đặt cọc giữ xe",time:"15p",status:"Converted",unread:0},{id:3,lead:"Đặng Ngọc Lan",agent:"Agent_01",last:"Giá tốt nhất là bao nhiêu?",time:"1g",status:"Active",unread:5},{id:4,lead:"Vũ Đình Tùng",agent:"Agent_02",last:"Cho em xin báo giá chi tiết",time:"3g",status:"Pending",unread:1}];
const POSTS=[{id:1,group:"Ô tô VN",content:"🔥 VinFast VF8 — Ưu đãi tháng 5! Giảm 50 triệu, hỗ trợ vay 0%...",time:"10g",likes:234,comments:67,shares:45,status:"Live"},{id:2,group:"Xe Miền Nam",content:"🎁 Mua xe VinFast nhận ngay phụ kiện chính hãng 20 triệu đồng...",time:"1 ngày",likes:189,comments:43,shares:28,status:"Live"},{id:3,group:"Xe Miền Bắc",content:"⚡ VF9 Premium — Xe điện sang trọng nhất thị trường 2025...",time:"2 ngày",likes:456,comments:112,shares:89,status:"Ended"}];
const CMTS=[{id:1,agent:"Agent_01",lead:"Nguyễn Văn An",post:"VF8 ưu đãi",comment:"Dạ anh ơi, còn màu đen và trắng ạ, mình inbox cho anh nhé!",time:"1g"},{id:2,agent:"Agent_02",lead:"Trần Thị Bích",post:"Mua xe nhận quà",comment:"Bạn Bích ơi mình tư vấn kỹ hơn qua inbox ạ!",time:"2g"},{id:3,agent:"Agent_01",lead:"Đặng Ngọc Lan",post:"VF8 ưu đãi",comment:"Chị Lan ơi, giá tốt nhất mình nhắn inbox ạ",time:"3g"}];
const STAFF0=[{id:1,name:"Nguyễn Hữu Đức",email:"duc@vinfast.vn",role:"Senior Sales",status:"Active",joined:"01/01/2025",convs:45,converted:12},{id:2,name:"Trần Minh Châu",email:"chau@vinfast.vn",role:"Sales",status:"Active",joined:"05/01/2025",convs:38,converted:9},{id:3,name:"Lê Thị Thanh",email:"thanh@vinfast.vn",role:"Sales",status:"Active",joined:"10/01/2025",convs:31,converted:11},{id:4,name:"Hoàng Văn Bình",email:"binh@vinfast.vn",role:"Junior Sales",status:"Suspended",joined:"15/01/2025",convs:22,converted:5}];
const ORGS_DB=[{id:1,name:"VinFast Sản Xuất",plan:"Enterprise",users:12,status:"Active",joined:"01/03/2025",rev:"₫6.9M"},{id:2,name:"Thế Giới Di Động",plan:"Pro",users:8,status:"Active",joined:"10/03/2025",rev:"₫2.9M"},{id:3,name:"FPT Software",plan:"Pro",users:5,status:"Active",joined:"15/03/2025",rev:"₫2.9M"},{id:4,name:"Vinamilk Group",plan:"Starter",users:3,status:"Active",joined:"20/03/2025",rev:"₫990K"},{id:5,name:"An Khang Pharmacy",plan:"Pro",users:6,status:"Suspended",joined:"01/04/2025",rev:"₫0"}];
const FILES=[{id:1,name:"product_catalog_2025.pdf",size:"2.4 MB",date:"20/04"},{id:2,name:"price_list_Q2.xlsx",size:"1.1 MB",date:"18/04"},{id:3,name:"faq_customers.txt",size:"156 KB",date:"15/04"}];
const ORGS=[{id:1,name:"VinFast Sản Xuất",abbr:"VF",plan:"Enterprise",color:"#4f46e5"},{id:2,name:"Thế Giới Di Động",abbr:"TG",plan:"Pro",color:"#0ea5e9"},{id:3,name:"FPT Software",abbr:"FP",plan:"Pro",color:"#10b981"}];

// ── STYLE HELPERS ─────────────────────────────────────────────────
const D={background:"#0d101a",color:"#e5e7eb",fontFamily:"system-ui,sans-serif"};
const card=(x={})=>({background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,padding:20,...x});
const inp={background:"#2a2f45",border:"1px solid #374151",borderRadius:9,padding:"10px 14px",color:"#fff",fontSize:13,outline:"none",width:"100%",boxSizing:"border-box"};
const PB=(p={})=>({padding:"10px 20px",borderRadius:9,border:"none",cursor:"pointer",fontSize:14,fontWeight:500,background:"#4f46e5",color:"#fff",...p});
const SB=(p={})=>({padding:"10px 20px",borderRadius:9,border:"1px solid #374151",cursor:"pointer",fontSize:13,background:"transparent",color:"#d1d5db",...p});
const sc=s=>({Hot:"#ef4444",Warm:"#f59e0b",Cold:"#3b82f6",Active:"#22c55e",Converted:"#6366f1",Pending:"#6b7280",Live:"#22c55e",Ended:"#6b7280",Suspended:"#ef4444",Enterprise:"#d97706",Pro:"#6366f1",Starter:"#9ca3af"}[s]||"#6b7280");

// ── MICRO COMPONENTS ──────────────────────────────────────────────
const Av=({t,bg="#4f46e5",sz=30})=><div style={{width:sz,height:sz,background:bg,borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center",color:"#fff",fontSize:sz*.36,fontWeight:700,flexShrink:0}}>{t}</div>;
const Bdg=({label})=><span style={{background:sc(label)+"22",color:sc(label),border:`1px solid ${sc(label)}44`,fontSize:11,fontWeight:500,padding:"2px 9px",borderRadius:99}}>{label}</span>;
const Lbl=({t})=><p style={{color:"#9ca3af",fontSize:12,marginBottom:5}}>{t}</p>;
const Inp=(p)=><input style={inp} {...p}/>;
const Row=(p)=><div style={{display:"flex",alignItems:"center",...p.style}} {...p}/>;

// ═════════════════════════════════════════════════════════════════
// LANDING PAGE
// ═════════════════════════════════════════════════════════════════
function Landing({onLogin,onRegister,onAdmin}){
  const feats=[{e:"⚡",t:"AI Agents 24/7",d:"Tự động tư vấn, chốt lead liên tục không cần nhân viên trực."},{e:"🏢",t:"Multi-Organization",d:"Một platform, nhiều tổ chức, dữ liệu hoàn toàn độc lập."},{e:"🎯",t:"Lead Scoring AI",d:"Chấm điểm và phân loại lead theo tỷ lệ chốt thực tế."},{e:"🏆",t:"KPI Leaderboard",d:"Admin tự cấu hình thưởng phạt, không cần coder."},{e:"🔒",t:"Private Data",d:"Upload tệp kinh doanh riêng, AI học theo sản phẩm của bạn."},{e:"📱",t:"Facebook Native",d:"Tích hợp trực tiếp Facebook, session bền vững."}];
  const plans=[{n:"Starter",p:"990K",f:["1 tổ chức","3 AI Agents","5 nhân viên","Email support"]},{n:"Pro",p:"2.9M",f:["3 tổ chức","10 AI Agents","20 nhân viên","Priority support","Custom branding"],hot:true},{n:"Enterprise",p:"Liên hệ",f:["Unlimited org","Unlimited agents","SLA 99.9%","Dedicated support"]}];
  return(
    <div style={{...D,minHeight:"100vh",overflowY:"auto"}}>
      {/* NAV */}
      <nav style={{display:"flex",alignItems:"center",padding:"15px 48px",borderBottom:"1px solid #1e2130",position:"sticky",top:0,background:"#0d101aee",zIndex:20}}>
        <Row style={{gap:10}}><div style={{width:32,height:32,background:"#4f46e5",borderRadius:8,display:"flex",alignItems:"center",justifyContent:"center"}}><Zap size={16} color="#fff"/></div><span style={{fontWeight:800,fontSize:17,color:"#fff"}}>AutoFlow</span></Row>
        <Row style={{gap:28,marginLeft:44}}>{["Tính năng","Bảng giá","Về chúng tôi"].map(l=><a key={l} href="#" style={{color:"#9ca3af",fontSize:13,textDecoration:"none"}}>{l}</a>)}</Row>
        <Row style={{marginLeft:"auto",gap:10}}>
          <button onClick={onLogin} style={SB({padding:"7px 18px"})}>Đăng nhập</button>
          <button onClick={onRegister} style={PB({padding:"7px 18px",fontWeight:700})}>Dùng thử miễn phí</button>
        </Row>
      </nav>
      {/* HERO */}
      <section style={{textAlign:"center",padding:"72px 24px 52px"}}>
        <div style={{display:"inline-block",background:"#312e8122",border:"1px solid #4f46e544",color:"#a5b4fc",fontSize:12,padding:"5px 14px",borderRadius:99,marginBottom:18}}>🚀 Facebook Automation Platform #1 Việt Nam</div>
        <h1 style={{fontSize:48,fontWeight:900,color:"#fff",lineHeight:1.15,margin:"0 auto 16px",maxWidth:680}}>Tự động hóa sales Facebook<br/><span style={{color:"#818cf8"}}>tăng doanh thu 3×</span></h1>
        <p style={{color:"#9ca3af",fontSize:16,maxWidth:500,margin:"0 auto 32px",lineHeight:1.8}}>AI agents làm việc 24/7, tự động tư vấn và chốt leads từ hàng nghìn nhóm Facebook.</p>
        <Row style={{gap:12,justifyContent:"center"}}>
          <button onClick={onRegister} style={PB({padding:"13px 30px",fontSize:15,fontWeight:800})}>Tạo tổ chức miễn phí →</button>
          <button style={SB({padding:"13px 30px",fontSize:15})}>Xem demo</button>
        </Row>
        {/* Mini app preview */}
        <div style={{maxWidth:740,margin:"48px auto 0",background:"#111520",border:"1px solid #1e2130",borderRadius:16,padding:14,textAlign:"left"}}>
          <Row style={{gap:5,marginBottom:10}}>{["#ef4444","#f59e0b","#22c55e"].map(c=><div key={c} style={{width:9,height:9,borderRadius:"50%",background:c}}/>)}</Row>
          <div style={{display:"flex",gap:10,height:150}}>
            <div style={{width:105,background:"#0d101a",borderRadius:8,padding:8}}>
              {["Leads","Inbox","Posting","Leaderboard","Settings"].map((t,i)=><div key={t} style={{padding:"5px 8px",borderRadius:5,background:i===0?"#4f46e5":"transparent",color:i===0?"#fff":"#6b7280",fontSize:10,marginBottom:3}}>{t}</div>)}
            </div>
            <div style={{flex:1,display:"flex",flexDirection:"column",gap:7}}>
              <div style={{display:"grid",gridTemplateColumns:"repeat(4,1fr)",gap:6}}>{[{l:"Total",v:"1,284"},{l:"Hot",v:"342"},{l:"Conv",v:"89"},{l:"Revenue",v:"₫4.2B"}].map(s=><div key={s.l} style={{background:"#1e2130",borderRadius:7,padding:"8px 10px"}}><p style={{color:"#6b7280",fontSize:8}}>{s.l}</p><p style={{color:"#fff",fontWeight:700,fontSize:13}}>{s.v}</p></div>)}</div>
              <div style={{flex:1,background:"#1e2130",borderRadius:7,padding:10}}>{LEADS.slice(0,3).map(l=><div key={l.id} style={{display:"flex",alignItems:"center",gap:7,marginBottom:6}}><div style={{width:18,height:18,background:"#4f46e5",borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center",fontSize:8,color:"#fff"}}>{l.name[0]}</div><span style={{color:"#d1d5db",fontSize:10,flex:1}}>{l.name}</span><span style={{background:sc(l.status)+"22",color:sc(l.status),fontSize:9,padding:"1px 6px",borderRadius:99}}>{l.status}</span><span style={{color:"#6b7280",fontSize:9}}>{l.score}</span></div>)}</div>
            </div>
          </div>
        </div>
      </section>
      {/* STATS */}
      <div style={{padding:"40px 48px",background:"#111520",borderTop:"1px solid #1e2130",borderBottom:"1px solid #1e2130"}}>
        <div style={{display:"grid",gridTemplateColumns:"repeat(4,1fr)",maxWidth:760,margin:"0 auto",textAlign:"center",gap:16}}>
          {[{v:"500+",l:"Tổ chức tin dùng"},{v:"50K+",l:"Leads/tháng"},{v:"98%",l:"Uptime"},{v:"3×",l:"Tăng doanh thu TB"}].map(s=><div key={s.l}><p style={{fontSize:30,fontWeight:900,color:"#818cf8"}}>{s.v}</p><p style={{color:"#9ca3af",fontSize:13,marginTop:4}}>{s.l}</p></div>)}
        </div>
      </div>
      {/* FEATURES */}
      <section style={{padding:"56px 48px",maxWidth:1040,margin:"0 auto"}}>
        <h2 style={{textAlign:"center",fontSize:32,fontWeight:800,color:"#fff",marginBottom:36}}>Tất cả trong một nền tảng</h2>
        <div style={{display:"grid",gridTemplateColumns:"repeat(3,1fr)",gap:14}}>
          {feats.map(f=><div key={f.t} style={{background:"#111520",border:"1px solid #1e2130",borderRadius:14,padding:22}}><span style={{fontSize:26}}>{f.e}</span><h3 style={{color:"#e5e7eb",fontSize:14,fontWeight:600,margin:"10px 0 6px"}}>{f.t}</h3><p style={{color:"#6b7280",fontSize:12,lineHeight:1.7}}>{f.d}</p></div>)}
        </div>
      </section>
      {/* PRICING */}
      <section style={{padding:"56px 48px",maxWidth:880,margin:"0 auto"}}>
        <h2 style={{textAlign:"center",fontSize:32,fontWeight:800,color:"#fff",marginBottom:36}}>Bảng giá minh bạch</h2>
        <div style={{display:"grid",gridTemplateColumns:"repeat(3,1fr)",gap:14}}>
          {plans.map(p=><div key={p.n} style={{background:"#111520",border:`1px solid ${p.hot?"#4f46e5":"#1e2130"}`,borderRadius:16,padding:24,position:"relative"}}>
            {p.hot&&<span style={{position:"absolute",top:-11,left:"50%",transform:"translateX(-50%)",background:"#4f46e5",color:"#fff",fontSize:10,fontWeight:700,padding:"3px 12px",borderRadius:99,whiteSpace:"nowrap"}}>Phổ biến nhất</span>}
            <p style={{color:p.hot?"#a5b4fc":"#9ca3af",fontWeight:700,fontSize:13}}>{p.n}</p>
            <p style={{color:"#fff",fontSize:28,fontWeight:900,margin:"8px 0"}}>{p.p}<span style={{fontSize:12,color:"#6b7280",fontWeight:400}}>{p.p!=="Liên hệ"?"/tháng":""}</span></p>
            <div style={{borderTop:"1px solid #1e2130",paddingTop:12,marginBottom:14}}>
              {p.f.map(f=><div key={f} style={{display:"flex",gap:7,alignItems:"center",marginBottom:8}}><Check size={12} color="#22c55e"/><span style={{color:"#d1d5db",fontSize:12}}>{f}</span></div>)}
            </div>
            <button onClick={onRegister} style={{width:"100%",padding:"10px",background:p.hot?"#4f46e5":"#1e2130",border:`1px solid ${p.hot?"#4f46e5":"#374151"}`,borderRadius:9,color:"#fff",fontSize:13,cursor:"pointer",fontWeight:p.hot?700:400}}>{p.n==="Enterprise"?"Liên hệ":"Bắt đầu ngay"}</button>
          </div>)}
        </div>
      </section>
      {/* FOOTER */}
      <footer style={{padding:"22px 48px",borderTop:"1px solid #1e2130",display:"flex",alignItems:"center",justifyContent:"space-between"}}>
        <Row style={{gap:8}}><Zap size={13} color="#4f46e5"/><span style={{color:"#6b7280",fontSize:12}}>AutoFlow © 2025. All rights reserved.</span></Row>
        <Row style={{gap:20}}>
          {["Điều khoản","Bảo mật","Liên hệ"].map(l=><a key={l} href="#" style={{color:"#6b7280",fontSize:12,textDecoration:"none"}}>{l}</a>)}
          <button onClick={onAdmin} style={{background:"none",border:"none",color:"#374151",fontSize:11,cursor:"pointer"}}>· Admin Portal</button>
        </Row>
      </footer>
    </div>
  );
}

// ═════════════════════════════════════════════════════════════════
// AUTH PAGES
// ═════════════════════════════════════════════════════════════════
function Auth({mode,setMode,onSuccess,goBack}){
  const [step,setStep]=useState(1);
  const [sent,setSent]=useState(false);
  const [pwOk,setPwOk]=useState(false);
  const box={maxWidth:460,margin:"48px auto",...card({padding:36})};
  const Back=()=><button onClick={()=>{setSent(false);setPwOk(false);setMode("login");}} style={{display:"flex",alignItems:"center",gap:6,background:"none",border:"none",color:"#9ca3af",fontSize:13,cursor:"pointer",marginBottom:22,padding:0}}><ArrowLeft size={13}/>Quay lại đăng nhập</button>;

  if(mode==="success") return(
    <div style={{...D,minHeight:"100vh",display:"flex",alignItems:"center",justifyContent:"center"}}>
      <div style={{...card({padding:44}),textAlign:"center",maxWidth:440}}>
        <div style={{width:72,height:72,background:"#16a34a22",border:"2px solid #16a34a",borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center",margin:"0 auto 18px"}}><Check size={34} color="#4ade80"/></div>
        <h2 style={{color:"#f9fafb",fontSize:21,fontWeight:700,marginBottom:8}}>Tổ chức đã được tạo!</h2>
        <p style={{color:"#9ca3af",fontSize:13,marginBottom:0}}>Workspace của bạn đã sẵn sàng sử dụng.</p>
        <div style={{background:"#111520",borderRadius:10,padding:16,margin:"20px 0",textAlign:"left"}}>
          {[{l:"Tổ chức",v:"VinFast Sản Xuất"},{l:"Gói",v:"Pro (Trial 14 ngày)"},{l:"Admin",v:"admin@vinfast.vn"},{l:"Org ID",v:"org_vf_2025_042"}].map(r=><div key={r.l} style={{display:"flex",justifyContent:"space-between",marginBottom:8}}><span style={{color:"#6b7280",fontSize:12}}>{r.l}</span><span style={{color:"#e5e7eb",fontSize:12,fontWeight:500}}>{r.v}</span></div>)}
        </div>
        <button onClick={()=>onSuccess("admin")} style={{...PB(),width:"100%",padding:"12px",fontSize:15}}>Vào workspace →</button>
      </div>
    </div>
  );

  if(mode==="forgot") return(
    <div style={{...D,minHeight:"100vh",display:"flex",alignItems:"center",justifyContent:"center"}}>
      <div style={box}>
        <Back/>
        {!sent?(
          <>
            <div style={{textAlign:"center",marginBottom:26}}>
              <div style={{width:48,height:48,background:"#312e8133",border:"1px solid #4f46e544",borderRadius:14,display:"flex",alignItems:"center",justifyContent:"center",margin:"0 auto 14px"}}><Mail size={22} color="#818cf8"/></div>
              <h2 style={{color:"#f9fafb",fontSize:19,fontWeight:700}}>Quên mật khẩu?</h2>
              <p style={{color:"#9ca3af",fontSize:13,marginTop:6}}>Nhập email — chúng tôi gửi link đặt lại ngay</p>
            </div>
            <Lbl t="Email tài khoản"/><Inp type="email" placeholder="you@company.com" style={{marginBottom:16}}/>
            <button onClick={()=>setSent(true)} style={{...PB(),width:"100%",padding:"11px"}}>Gửi link đặt lại</button>
          </>
        ):(
          <div style={{textAlign:"center"}}>
            <div style={{width:58,height:58,background:"#16a34a22",border:"2px solid #16a34a55",borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center",margin:"0 auto 18px"}}><Check size={26} color="#4ade80"/></div>
            <h3 style={{color:"#f9fafb",fontSize:17,fontWeight:600,marginBottom:8}}>Email đã được gửi!</h3>
            <p style={{color:"#9ca3af",fontSize:13,marginBottom:20}}>Kiểm tra hộp thư và nhấn link để đặt lại mật khẩu. Hiệu lực 30 phút.</p>
            <button onClick={()=>setMode("login")} style={SB({fontSize:13})}>Quay lại đăng nhập</button>
          </div>
        )}
      </div>
    </div>
  );

  if(mode==="login") return(
    <div style={{...D,minHeight:"100vh",display:"flex",alignItems:"center",justifyContent:"center"}}>
      <div style={box}>
        <button onClick={goBack} style={{display:"flex",alignItems:"center",gap:6,background:"none",border:"none",color:"#9ca3af",fontSize:12,cursor:"pointer",marginBottom:22,padding:0}}><ArrowLeft size={13}/>Trang chủ</button>
        <div style={{textAlign:"center",marginBottom:26}}>
          <div style={{width:40,height:40,background:"#4f46e5",borderRadius:10,display:"flex",alignItems:"center",justifyContent:"center",margin:"0 auto 12px"}}><Zap size={18} color="#fff"/></div>
          <h2 style={{color:"#f9fafb",fontSize:20,fontWeight:700}}>Đăng nhập AutoFlow</h2>
        </div>
        <Lbl t="Email"/><Inp type="email" defaultValue="admin@vinfast.vn" style={{marginBottom:14}}/>
        <Lbl t="Mật khẩu"/><Inp type="password" defaultValue="password123"/>
        <div style={{display:"flex",justifyContent:"flex-end",margin:"8px 0 18px"}}>
          <button onClick={()=>setMode("forgot")} style={{background:"none",border:"none",color:"#818cf8",fontSize:12,cursor:"pointer"}}>Quên mật khẩu?</button>
        </div>
        <button onClick={()=>onSuccess("admin")} style={{...PB(),width:"100%",padding:"12px",fontSize:14,fontWeight:700}}>Đăng nhập</button>
        <p style={{textAlign:"center",color:"#6b7280",fontSize:13,marginTop:18}}>Chưa có tài khoản? <button onClick={()=>setMode("register")} style={{background:"none",border:"none",color:"#818cf8",cursor:"pointer",fontSize:13}}>Tạo tổ chức</button></p>
        <div style={{borderTop:"1px solid #2a2f45",marginTop:18,paddingTop:16,textAlign:"center"}}>
          <p style={{color:"#6b7280",fontSize:12,marginBottom:10}}>Đăng nhập với tư cách nhân viên</p>
          <button onClick={()=>onSuccess("staff")} style={SB({fontSize:13,padding:"8px 20px"})}>Staff login →</button>
        </div>
      </div>
    </div>
  );

  // REGISTER
  return(
    <div style={{...D,minHeight:"100vh",display:"flex",alignItems:"center",justifyContent:"center"}}>
      <div style={{...box,maxWidth:520}}>
        <button onClick={goBack} style={{display:"flex",alignItems:"center",gap:6,background:"none",border:"none",color:"#9ca3af",fontSize:12,cursor:"pointer",marginBottom:20,padding:0}}><ArrowLeft size={13}/>Trang chủ</button>
        <Row style={{gap:8,marginBottom:26,justifyContent:"center"}}>
          {[1,2].map(s=><><div key={s} style={{width:28,height:28,borderRadius:"50%",background:step>=s?"#4f46e5":"#2a2f45",display:"flex",alignItems:"center",justifyContent:"center",color:"#fff",fontSize:13,fontWeight:600}}>{step>s?<Check size={13}/>:s}</div>{s<2&&<div style={{width:40,height:2,background:step>1?"#4f46e5":"#2a2f45"}}/>}</>)}
        </Row>
        {step===1?(
          <>
            <h2 style={{color:"#f9fafb",fontSize:19,fontWeight:700,marginBottom:4}}>Tạo tài khoản</h2>
            <p style={{color:"#9ca3af",fontSize:13,marginBottom:22}}>Bước 1: Thông tin cá nhân</p>
            <div style={{display:"grid",gridTemplateColumns:"1fr 1fr",gap:13,marginBottom:13}}>
              <div><Lbl t="Họ và tên"/><Inp placeholder="Nguyễn Văn A"/></div>
              <div><Lbl t="Email"/><Inp type="email" placeholder="you@company.com"/></div>
              <div><Lbl t="Mật khẩu"/><Inp type="password" placeholder="Tối thiểu 8 ký tự"/></div>
              <div><Lbl t="Xác nhận mật khẩu"/><Inp type="password" placeholder="Nhập lại"/></div>
            </div>
            <button onClick={()=>setStep(2)} style={{...PB(),width:"100%",padding:"11px"}}>Tiếp theo →</button>
          </>
        ):(
          <>
            <h2 style={{color:"#f9fafb",fontSize:19,fontWeight:700,marginBottom:4}}>Tạo tổ chức</h2>
            <p style={{color:"#9ca3af",fontSize:13,marginBottom:22}}>Bước 2: Thông tin tổ chức</p>
            <div style={{display:"grid",gridTemplateColumns:"1fr 1fr",gap:13,marginBottom:13}}>
              <div style={{gridColumn:"1/-1"}}><Lbl t="Tên tổ chức"/><Inp placeholder="VinFast Sản Xuất"/></div>
              <div><Lbl t="Lĩnh vực"/><select style={{...inp}}><option>Sản xuất</option><option>Bán lẻ</option><option>Công nghệ</option><option>Bất động sản</option><option>Khác</option></select></div>
              <div><Lbl t="Số nhân viên"/><select style={{...inp}}><option>1-5</option><option>6-20</option><option>21-50</option><option>50+</option></select></div>
              <div><Lbl t="Gói dịch vụ"/><select style={{...inp}}><option>Starter — 990K/tháng</option><option>Pro — 2.9M/tháng</option><option>Enterprise</option></select></div>
              <div><Lbl t="Mã giới thiệu"/><Inp placeholder="REF-XXXX (nếu có)"/></div>
            </div>
            <button onClick={()=>setMode("success")} style={{...PB(),width:"100%",padding:"11px",fontWeight:700}}>Tạo tổ chức →</button>
            <button onClick={()=>setStep(1)} style={{display:"block",background:"none",border:"none",color:"#9ca3af",fontSize:12,cursor:"pointer",margin:"12px auto 0"}}>← Quay lại</button>
          </>
        )}
      </div>
    </div>
  );
}

// ═════════════════════════════════════════════════════════════════
// SUPER ADMIN
// ═════════════════════════════════════════════════════════════════
function SuperAdmin({goBack}){
  const [atab,setAtab]=useState("orgs");
  return(
    <div style={{...D,minHeight:"100vh"}}>
      <div style={{display:"flex",alignItems:"center",padding:"13px 24px",background:"#111520",borderBottom:"1px solid #1e2130"}}>
        <Row style={{gap:10}}>
          <div style={{width:30,height:30,background:"#dc2626",borderRadius:8,display:"flex",alignItems:"center",justifyContent:"center"}}><Shield size={14} color="#fff"/></div>
          <span style={{fontWeight:700,fontSize:14,color:"#fff"}}>AutoFlow Admin Portal</span>
          <span style={{background:"#dc262622",color:"#f87171",border:"1px solid #dc262644",fontSize:10,padding:"2px 8px",borderRadius:99}}>Super Admin</span>
        </Row>
        <button onClick={goBack} style={{...SB({padding:"6px 14px",fontSize:12}),marginLeft:"auto"}}>← Trang chủ</button>
      </div>
      <div style={{padding:22}}>
        <div style={{display:"grid",gridTemplateColumns:"repeat(4,1fr)",gap:12,marginBottom:22}}>
          {[{l:"Tổng tổ chức",v:"5",c:"#818cf8"},{l:"Người dùng",v:"34",c:"#4ade80"},{l:"Doanh thu/tháng",v:"₫18.5M",c:"#fbbf24"},{l:"Sessions live",v:"12",c:"#38bdf8"}].map(s=><div key={s.l} style={card()}><p style={{color:"#6b7280",fontSize:11}}>{s.l}</p><p style={{fontSize:24,fontWeight:800,color:s.c,marginTop:4}}>{s.v}</p></div>)}
        </div>
        <Row style={{gap:8,marginBottom:18}}>
          {[["orgs","Tổ chức"],["users","Người dùng"],["system","Hệ thống"]].map(([id,l])=><button key={id} onClick={()=>setAtab(id)} style={{padding:"7px 16px",borderRadius:8,border:"none",cursor:"pointer",fontSize:13,background:atab===id?"#4f46e5":"#1e2130",color:atab===id?"#fff":"#9ca3af"}}>{l}</button>)}
        </Row>
        {atab==="orgs"&&<div style={{background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflow:"hidden"}}>
          <table style={{width:"100%",borderCollapse:"collapse",fontSize:13}}>
            <thead><tr style={{borderBottom:"1px solid #2a2f45"}}>{["Tổ chức","Gói","Users","Doanh thu","Ngày tạo","Status",""].map(h=><th key={h} style={{padding:"10px 14px",textAlign:"left",color:"#6b7280",fontWeight:500,fontSize:12}}>{h}</th>)}</tr></thead>
            <tbody>{ORGS_DB.map(o=><tr key={o.id} style={{borderBottom:"1px solid #1a1f35"}}>
              <td style={{padding:"10px 14px"}}><Row style={{gap:8}}><Av t={o.name[0]} sz={26}/><span style={{color:"#e5e7eb",fontWeight:500}}>{o.name}</span></Row></td>
              <td style={{padding:"10px 14px"}}><Bdg label={o.plan}/></td>
              <td style={{padding:"10px 14px",color:"#d1d5db"}}>{o.users}</td>
              <td style={{padding:"10px 14px",color:"#fbbf24",fontWeight:500}}>{o.rev}</td>
              <td style={{padding:"10px 14px",color:"#6b7280"}}>{o.joined}</td>
              <td style={{padding:"10px 14px"}}><Bdg label={o.status}/></td>
              <td style={{padding:"10px 14px"}}><button style={{background:"none",border:"1px solid #374151",borderRadius:6,color:"#9ca3af",fontSize:11,padding:"4px 10px",cursor:"pointer"}}>Quản lý</button></td>
            </tr>)}</tbody>
          </table>
        </div>}
        {atab==="users"&&<div style={card()}><p style={{color:"#9ca3af",fontSize:13,marginBottom:14}}>Toàn bộ user accounts trong hệ thống.</p>{STAFF0.map(s=><div key={s.id} style={{display:"flex",alignItems:"center",gap:12,padding:"10px 0",borderBottom:"1px solid #2a2f45"}}><Av t={s.name[0]} sz={28}/><div style={{flex:1}}><p style={{color:"#e5e7eb",fontWeight:500,fontSize:13}}>{s.name}</p><p style={{color:"#6b7280",fontSize:12}}>{s.email}</p></div><Bdg label={s.status}/></div>)}</div>}
        {atab==="system"&&<div style={{display:"grid",gridTemplateColumns:"1fr 1fr",gap:14}}>{[{t:"Database",v:"PostgreSQL v16"},{t:"Redis Cache",v:"v7.2"},{t:"AI Service",v:"GPT-4o"},{t:"FB Session Server",v:"Node.js v21"}].map(s=><div key={s.t} style={card({display:"flex",alignItems:"center",justifyContent:"space-between"})}><div><p style={{color:"#9ca3af",fontSize:12}}>{s.t}</p><p style={{color:"#e5e7eb",fontWeight:500,fontSize:14}}>{s.v}</p></div><Bdg label="Active"/></div>)}</div>}
      </div>
    </div>
  );
}

// ═════════════════════════════════════════════════════════════════
// SETTINGS (full - brand / security / staff / agents / billing)
// ═════════════════════════════════════════════════════════════════
function SettingsPage({org}){
  const [st,setSt]=useState("brand");
  const [staff,setStaff]=useState(STAFF0);
  const [showAdd,setShowAdd]=useState(false);
  const [ns,setNs]=useState({name:"",email:"",role:"Sales"});
  const [pwOk,setPwOk]=useState(false);
  const [color,setColor]=useState(org.color||"#4f46e5");
  const [abbr,setAbbr]=useState(org.abbr||"VF");
  const tabs=[{id:"brand",l:"Thương hiệu",I:Palette},{id:"security",l:"Bảo mật",I:Shield},{id:"staff",l:"Nhân viên",I:Users},{id:"agents",l:"AI Agents",I:Zap},{id:"billing",l:"Thanh toán",I:CreditCard}];
  const addS=()=>{if(!ns.name||!ns.email)return;setStaff(p=>[...p,{id:Date.now(),...ns,status:"Active",joined:new Date().toLocaleDateString("vi"),convs:0,converted:0}]);setNs({name:"",email:"",role:"Sales"});setShowAdd(false);};
  return(
    <div>
      <div style={{display:"flex",gap:6,marginBottom:22,flexWrap:"wrap"}}>
        {tabs.map(({id,l,I})=><button key={id} onClick={()=>setSt(id)} style={{display:"flex",alignItems:"center",gap:6,padding:"7px 13px",borderRadius:9,border:"none",cursor:"pointer",fontSize:12,background:st===id?"#4f46e5":"#1e2130",color:st===id?"#fff":"#9ca3af"}}><I size={12}/>{l}</button>)}
      </div>

      {/* BRANDING */}
      {st==="brand"&&<div style={{display:"flex",flexDirection:"column",gap:14}}>
        <div style={card()}>
          <p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:18}}>Nhận diện thương hiệu</p>
          <div style={{display:"grid",gridTemplateColumns:"auto 1fr",gap:22,alignItems:"start"}}>
            <div style={{textAlign:"center"}}>
              <div style={{width:80,height:80,background:color,borderRadius:18,display:"flex",alignItems:"center",justifyContent:"center",color:"#fff",fontSize:28,fontWeight:900,marginBottom:10,border:"3px solid "+color+"44"}}>{abbr}</div>
              <p style={{color:"#6b7280",fontSize:11}}>Avatar / Logo</p>
              <button style={{...SB({padding:"4px 10px",fontSize:11}),marginTop:6}}>Upload</button>
            </div>
            <div style={{display:"grid",gridTemplateColumns:"1fr 1fr",gap:12}}>
              <div><Lbl t="Tên tổ chức"/><Inp defaultValue={org.name}/></div>
              <div><Lbl t="Viết tắt (2–3 ký tự)"/><Inp value={abbr} onChange={e=>setAbbr(e.target.value.slice(0,3).toUpperCase())} placeholder="VF"/></div>
              <div><Lbl t="Màu thương hiệu"/>
                <div style={{display:"flex",gap:8,alignItems:"center"}}>
                  <input type="color" value={color} onChange={e=>setColor(e.target.value)} style={{width:38,height:34,border:"none",borderRadius:8,cursor:"pointer",background:"none"}}/>
                  <Inp value={color} onChange={e=>setColor(e.target.value)} style={{...inp,flex:1}}/>
                </div>
              </div>
              <div><Lbl t="Ngành"/><select style={{...inp}}><option>Sản xuất</option><option>Bán lẻ</option><option>Công nghệ</option><option>Bất động sản</option></select></div>
            </div>
          </div>
        </div>
        <div style={card()}>
          <p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:14}}>Upload logo đầy đủ</p>
          <div style={{border:"2px dashed #374151",borderRadius:12,padding:30,textAlign:"center"}}>
            <Upload size={26} color="#6b7280" style={{margin:"0 auto 10px",display:"block"}}/>
            <p style={{color:"#9ca3af",fontSize:12,marginBottom:12}}>PNG, SVG khuyến nghị · Nền trong suốt</p>
            <button style={SB({fontSize:12,padding:"6px 14px"})}>Chọn file</button>
          </div>
        </div>
        <button style={{...PB({padding:"10px 24px"}),alignSelf:"flex-end"}}>Lưu thay đổi</button>
      </div>}

      {/* SECURITY */}
      {st==="security"&&<div style={{display:"flex",flexDirection:"column",gap:14,maxWidth:480}}>
        <div style={card()}>
          <p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:18}}>Đổi mật khẩu</p>
          {!pwOk?(<>
            <Lbl t="Mật khẩu hiện tại"/><Inp type="password" placeholder="••••••••" style={{marginBottom:13}}/>
            <Lbl t="Mật khẩu mới"/><Inp type="password" placeholder="Tối thiểu 8 ký tự, có chữ hoa và số" style={{marginBottom:13}}/>
            <Lbl t="Xác nhận mật khẩu mới"/><Inp type="password" placeholder="Nhập lại mật khẩu mới" style={{marginBottom:18}}/>
            <button onClick={()=>setPwOk(true)} style={PB({padding:"10px 22px"})}>Cập nhật mật khẩu</button>
          </>):(
            <div style={{display:"flex",alignItems:"center",gap:12,padding:14,background:"#16a34a22",border:"1px solid #16a34a44",borderRadius:10}}>
              <Check size={18} color="#4ade80"/>
              <div><p style={{color:"#4ade80",fontWeight:500,fontSize:13}}>Mật khẩu đã được cập nhật!</p><button onClick={()=>setPwOk(false)} style={{background:"none",border:"none",color:"#9ca3af",fontSize:11,cursor:"pointer",padding:0,marginTop:3}}>Đổi lại</button></div>
            </div>
          )}
        </div>
        <div style={card()}>
          <p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:10}}>Quên mật khẩu</p>
          <p style={{color:"#9ca3af",fontSize:12,marginBottom:13}}>Gửi link reset về email admin của tổ chức.</p>
          <button style={SB({fontSize:12,padding:"8px 16px"})}>Gửi link đặt lại →</button>
        </div>
        <div style={card()}>
          <p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:13}}>Phiên đăng nhập</p>
          {[{d:"Chrome · Ho Chi Minh City",t:"Đang hoạt động",a:true},{d:"Safari Mobile · Hà Nội",t:"3 giờ trước",a:false}].map((s,i)=><div key={i} style={{display:"flex",alignItems:"center",justifyContent:"space-between",padding:"9px 0",borderBottom:"1px solid #2a2f45"}}>
            <div><p style={{color:"#e5e7eb",fontSize:13}}>{s.d}</p><p style={{color:s.a?"#4ade80":"#6b7280",fontSize:11}}>{s.t}</p></div>
            {!s.a&&<button style={{background:"none",border:"1px solid #374151",borderRadius:6,color:"#f87171",fontSize:11,padding:"4px 10px",cursor:"pointer"}}>Thu hồi</button>}
          </div>)}
        </div>
      </div>}

      {/* STAFF */}
      {st==="staff"&&<div style={{display:"flex",flexDirection:"column",gap:14}}>
        <div style={{display:"flex",alignItems:"center",justifyContent:"space-between"}}>
          <p style={{color:"#9ca3af",fontSize:13}}>{staff.length} nhân viên · Gói Pro (tối đa 20)</p>
          <button onClick={()=>setShowAdd(!showAdd)} style={{...PB({padding:"8px 15px",fontSize:12}),display:"flex",alignItems:"center",gap:6}}><UserPlus size={13}/>Thêm nhân viên</button>
        </div>
        {showAdd&&<div style={card({border:"1px solid #4f46e544"})}>
          <p style={{color:"#a5b4fc",fontWeight:500,fontSize:13,marginBottom:14}}>Thêm nhân viên mới</p>
          <div style={{display:"grid",gridTemplateColumns:"1fr 1fr 1fr auto",gap:10,alignItems:"end"}}>
            <div><Lbl t="Họ tên"/><Inp value={ns.name} onChange={e=>setNs(p=>({...p,name:e.target.value}))} placeholder="Nguyễn Văn A"/></div>
            <div><Lbl t="Email"/><Inp value={ns.email} onChange={e=>setNs(p=>({...p,email:e.target.value}))} placeholder="nva@company.vn"/></div>
            <div><Lbl t="Vai trò"/><select value={ns.role} onChange={e=>setNs(p=>({...p,role:e.target.value}))} style={{...inp}}><option>Sales</option><option>Senior Sales</option><option>Team Lead</option></select></div>
            <button onClick={addS} style={PB({padding:"10px 14px"})}>Thêm</button>
          </div>
          <p style={{color:"#6b7280",fontSize:11,marginTop:9}}>Nhân viên nhận email mời, tự đặt mật khẩu lần đầu đăng nhập.</p>
        </div>}
        <div style={{background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflow:"hidden"}}>
          <table style={{width:"100%",borderCollapse:"collapse",fontSize:12}}>
            <thead><tr style={{borderBottom:"1px solid #2a2f45"}}>{["Nhân viên","Email","Vai trò","Convs","Conv Rate","Tham gia","Status",""].map(h=><th key={h} style={{padding:"10px 13px",textAlign:"left",color:"#6b7280",fontWeight:500,fontSize:11}}>{h}</th>)}</tr></thead>
            <tbody>{staff.map(s=><tr key={s.id} style={{borderBottom:"1px solid #1a1f35"}}>
              <td style={{padding:"10px 13px"}}><Row style={{gap:8}}><Av t={s.name[0]} sz={26}/><span style={{color:"#e5e7eb",fontWeight:500}}>{s.name}</span></Row></td>
              <td style={{padding:"10px 13px",color:"#9ca3af"}}>{s.email}</td>
              <td style={{padding:"10px 13px",color:"#d1d5db"}}>{s.role}</td>
              <td style={{padding:"10px 13px",color:"#d1d5db"}}>{s.convs}</td>
              <td style={{padding:"10px 13px",color:"#4ade80"}}>{s.converted}</td>
              <td style={{padding:"10px 13px",color:"#6b7280"}}>{s.joined}</td>
              <td style={{padding:"10px 13px"}}><Bdg label={s.status}/></td>
              <td style={{padding:"10px 13px"}}><Row style={{gap:6}}>
                <button onClick={()=>setStaff(p=>p.map(x=>x.id===s.id?{...x,status:x.status==="Active"?"Suspended":"Active"}:x))} style={{background:"none",border:"1px solid #374151",borderRadius:6,color:"#9ca3af",fontSize:10,padding:"3px 8px",cursor:"pointer"}}>{s.status==="Active"?"Tạm dừng":"Kích hoạt"}</button>
                <button onClick={()=>setStaff(p=>p.filter(x=>x.id!==s.id))} style={{background:"none",border:"none",cursor:"pointer",color:"#6b7280"}}><X size={13}/></button>
              </Row></td>
            </tr>)}</tbody>
          </table>
        </div>
      </div>}

      {/* AGENTS */}
      {st==="agents"&&<div style={card()}><p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:14}}>AI Agents đang chạy</p>
        {["Agent_01","Agent_02","Agent_03"].map(a=><div key={a} style={{display:"flex",alignItems:"center",justifyContent:"space-between",padding:"11px 0",borderBottom:"1px solid #2a2f45"}}>
          <Row style={{gap:10}}><div style={{width:8,height:8,background:"#4ade80",borderRadius:"50%"}}/><div><p style={{color:"#d1d5db",fontSize:13,fontWeight:500}}>{a}</p><p style={{color:"#6b7280",fontSize:11}}>GPT-4o · 3 nhóm · 120 messages/ngày</p></div></Row>
          <button style={{background:"none",border:"1px solid #374151",borderRadius:8,color:"#818cf8",fontSize:12,padding:"6px 13px",cursor:"pointer"}}>Cấu hình</button>
        </div>)}
        <button style={{...PB({padding:"9px 16px",fontSize:12}),marginTop:14,display:"flex",alignItems:"center",gap:6}}><Plus size={13}/>Thêm Agent</button>
      </div>}

      {/* BILLING */}
      {st==="billing"&&<div style={{display:"flex",flexDirection:"column",gap:14}}>
        <div style={card({border:"1px solid #4f46e544"})}>
          <div style={{display:"flex",alignItems:"center",justifyContent:"space-between",marginBottom:14}}><div><p style={{color:"#9ca3af",fontSize:11,marginBottom:3}}>Gói hiện tại</p><p style={{color:"#a5b4fc",fontSize:18,fontWeight:700}}>Pro Plan</p></div><Bdg label="Active"/></div>
          {[{l:"Chu kỳ",v:"Tháng"},{l:"Ngày gia hạn",v:"01/06/2025"},{l:"Phương thức",v:"Chuyển khoản ngân hàng"}].map(r=><div key={r.l} style={{display:"flex",justifyContent:"space-between",padding:"8px 0",borderBottom:"1px solid #2a2f45"}}><span style={{color:"#6b7280",fontSize:13}}>{r.l}</span><span style={{color:"#e5e7eb",fontSize:13}}>{r.v}</span></div>)}
          <button style={{...PB(),width:"100%",padding:"11px",marginTop:16,fontSize:13}}>Nâng cấp lên Enterprise</button>
        </div>
        <div style={card()}><p style={{color:"#e5e7eb",fontWeight:600,fontSize:13,marginBottom:14}}>Mức sử dụng tháng này</p>
          {[{l:"AI Messages",c:8400,m:10000},{l:"Leads",c:284,m:500},{l:"Nhân viên",c:4,m:20}].map(u=><div key={u.l} style={{marginBottom:13}}>
            <div style={{display:"flex",justifyContent:"space-between",marginBottom:5}}><span style={{color:"#9ca3af",fontSize:12}}>{u.l}</span><span style={{color:"#e5e7eb",fontSize:12}}>{u.c.toLocaleString()} / {u.m.toLocaleString()}</span></div>
            <div style={{height:5,background:"#2a2f45",borderRadius:99}}><div style={{width:`${Math.round(u.c/u.m*100)}%`,height:"100%",background:u.c/u.m>0.85?"#ef4444":"#6366f1",borderRadius:99}}/></div>
          </div>)}
        </div>
      </div>}
    </div>
  );
}

// ═════════════════════════════════════════════════════════════════
// MAIN APP
// ═════════════════════════════════════════════════════════════════
function MainApp({role,goLanding}){
  const [tab,setTab]=useState("leads");
  const [org,setOrg]=useState(ORGS[0]);
  const [orgMenu,setOrgMenu]=useState(false);
  const [lf,setLf]=useState("All");
  const [conn,setConn]=useState(false);loading
  const [loading,setLoading]=useState(false);
  const [athr,setAthr]=useState(THREADS[0]);
  const [cfg,setCfg]=useState({conv:10,conv2:50,cmt:2,bonus:1000,bonusAmt:500000,pen:300,penAmt:100000});
  const [showCfg,setShowCfg]=useState(false);
  const [drag,setDrag]=useState(false);

  const isAdmin=role==="admin";
  const TABS=isAdmin
    ?[{id:"leads",l:"Leads",I:Users,b:null},{id:"browser",l:"Browser",I:Globe,b:null},{id:"inbox",l:"Inbox",I:MessageSquare,b:8},{id:"posting",l:"Posting",I:FileText,b:null},{id:"commenting",l:"Commenting",I:MessageCircle,b:null},{id:"leaderboard",l:"Leaderboard",I:Trophy,b:null},{id:"data",l:"Data Private",I:Database,b:null}]
    :[{id:"leads",l:"My Leads",I:Users,b:null},{id:"inbox",l:"Inbox",I:MessageSquare,b:3},{id:"leaderboard",l:"Leaderboard",I:Trophy,b:null},{id:"data",l:"Data Private",I:Database,b:null}];

  const connect=()=>{setLoading(true);setTimeout(()=>{setLoading(false);setConn(true);},1800);};
  const llist=lf==="All"?LEADS:LEADS.filter(l=>l.status===lf);
  const scored=[...STAFF0].map(s=>({...s,pts:s.convs*cfg.conv+s.converted*cfg.conv2+s.cmts*(cfg.cmt||2)||s.convs*cfg.conv+s.converted*cfg.conv2})).sort((a,b)=>b.pts-a.pts);

  const tabLabel={leads:isAdmin?"Leads":"My Leads",browser:"Browser",inbox:"Inbox",posting:"Posting",commenting:"Commenting",leaderboard:"Leaderboard",data:"Data Private",settings:"Settings"}[tab]||tab;

  const Content=()=>{
    if(tab==="leads") return(
      <div style={{display:"flex",flexDirection:"column",gap:14}}>
        <div style={{display:"grid",gridTemplateColumns:"repeat(4,1fr)",gap:11}}>
          {[{l:"Total Leads",v:LEADS.length,c:"#fff"},{l:"Hot Leads",v:LEADS.filter(l=>l.status==="Hot").length,c:"#f87171"},{l:"Warm Leads",v:LEADS.filter(l=>l.status==="Warm").length,c:"#fbbf24"},{l:"Avg Score",v:Math.round(LEADS.reduce((a,l)=>a+l.score,0)/LEADS.length),c:"#818cf8"}].map(s=><div key={s.l} style={card()}><p style={{color:"#6b7280",fontSize:11,marginBottom:4}}>{s.l}</p><p style={{fontSize:22,fontWeight:700,color:s.c}}>{s.v}</p></div>)}
        </div>
        <Row style={{gap:8}}>
          {["All","Hot","Warm","Cold"].map(x=><button key={x} onClick={()=>setLf(x)} style={{padding:"5px 12px",borderRadius:7,border:"none",cursor:"pointer",fontSize:12,background:lf===x?"#4f46e5":"#1e2130",color:lf===x?"#fff":"#9ca3af"}}>{x}</button>)}
          <button style={{...PB({padding:"6px 13px",fontSize:12}),marginLeft:"auto",display:"flex",alignItems:"center",gap:5}}><Plus size={13}/>Thêm lead</button>
        </Row>
        <div style={{background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflow:"hidden"}}>
          <table style={{width:"100%",borderCollapse:"collapse",fontSize:12}}>
            <thead><tr style={{borderBottom:"1px solid #2a2f45"}}>{["Khách hàng","Facebook","Status","Nhóm","Agent","Score","Liên hệ cuối"].map(h=><th key={h} style={{padding:"9px 14px",textAlign:"left",color:"#6b7280",fontWeight:500,fontSize:11}}>{h}</th>)}</tr></thead>
            <tbody>{llist.map(l=><tr key={l.id} style={{borderBottom:"1px solid #1a1f35"}}>
              <td style={{padding:"9px 14px"}}><Row style={{gap:8}}><Av t={l.name.split(" ").pop()[0]} sz={26}/><div><p style={{color:"#e5e7eb",fontWeight:500}}>{l.name}</p><p style={{color:"#6b7280",fontSize:10}}>{l.phone}</p></div></Row></td>
              <td style={{padding:"9px 14px",color:"#818cf8",fontSize:11}}>fb.com/...</td>
              <td style={{padding:"9px 14px"}}><Bdg label={l.status}/></td>
              <td style={{padding:"9px 14px",color:"#d1d5db"}}>{l.group}</td>
              <td style={{padding:"9px 14px"}}><span style={{background:"#2a2f45",color:"#d1d5db",padding:"2px 7px",borderRadius:5,fontSize:10}}>{l.agent}</span></td>
              <td style={{padding:"9px 14px"}}><Row style={{gap:7}}><div style={{width:40,height:4,background:"#374151",borderRadius:99,overflow:"hidden"}}><div style={{width:`${l.score}%`,height:"100%",background:"#6366f1"}}/></div><span style={{color:"#fff",fontWeight:600}}>{l.score}</span></Row></td>
              <td style={{padding:"9px 14px",color:"#6b7280"}}>{l.last}</td>
            </tr>)}</tbody>
          </table>
        </div>
      </div>
    );

    if(tab==="browser") return(
      <div style={{display:"flex",flexDirection:"column",gap:14}}>
        <div style={{display:"flex",alignItems:"center",gap:10,padding:"11px 14px",borderRadius:11,border:"1px solid",borderColor:conn?"#16a34a55":"#2a2f45",background:conn?"#052e1611":"#1e2130"}}>
          <div style={{width:7,height:7,borderRadius:"50%",background:conn?"#4ade80":"#6b7280"}}/>
          <p style={{fontSize:13,color:conn?"#4ade80":"#9ca3af",fontWeight:500}}>{conn?"Session đang hoạt động — Facebook kết nối thành công":"Chưa có session — Vui lòng kết nối Facebook"}</p>
          {conn&&<span style={{marginLeft:"auto",fontSize:11,color:"#4ade80",background:"#052e1633",border:"1px solid #16a34a44",padding:"3px 9px",borderRadius:7}}>Expires: 7 ngày</span>}
        </div>
        <div style={{background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflow:"hidden"}}>
          <div style={{display:"flex",alignItems:"center",gap:7,padding:"9px 14px",background:"#151824",borderBottom:"1px solid #2a2f45"}}>
            <Row style={{gap:5}}>{["#ef4444","#f59e0b","#22c55e"].map(c=><div key={c} style={{width:10,height:10,borderRadius:"50%",background:c}}/>)}</Row>
            <div style={{flex:1,display:"flex",alignItems:"center",gap:7,background:"#1e2130",borderRadius:7,padding:"4px 11px",margin:"0 10px"}}><span style={{fontSize:12}}>🔒</span><span style={{color:"#6b7280",fontSize:12}}>https://www.facebook.com</span></div>
            <button onClick={()=>setConn(false)} style={{background:"none",border:"none",color:"#6b7280",fontSize:11,cursor:"pointer"}}>Reset</button>
          </div>
          <div style={{minHeight:240,display:"flex",alignItems:"center",justifyContent:"center",padding:28}}>
            {!conn?(<div style={{textAlign:"center"}}>
              <div style={{width:60,height:60,background:"#1877f2",borderRadius:16,display:"flex",alignItems:"center",justifyContent:"center",margin:"0 auto 14px"}}><span style={{color:"#fff",fontSize:28,fontWeight:900}}>f</span></div>
              <p style={{color:"#f9fafb",fontWeight:600,fontSize:16,marginBottom:7}}>Kết nối tài khoản Facebook</p>
              <p style={{color:"#9ca3af",fontSize:13,marginBottom:22,maxWidth:260}}>Đăng nhập để AutoFlow thu thập leads và chạy AI agents trong các nhóm của bạn</p>
              <button onClick={connect} disabled={loading} style={{...PB({background:loading?"#374151":"#1877f2"}),display:"inline-flex",alignItems:"center",gap:7}}>
                {loading?<><RefreshCw size={14} style={{animation:"spin 1s linear infinite"}}/>Đang kết nối...</>:<><LogIn size={14}/>Đăng nhập Facebook</>}
              </button>
            </div>):(
              <div style={{textAlign:"center"}}>
                <div style={{width:60,height:60,background:"#16a34a",borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center",margin:"0 auto 14px"}}><Check size={28} color="#fff"/></div>
                <p style={{color:"#f9fafb",fontWeight:600,fontSize:16,marginBottom:5}}>Kết nối thành công!</p>
                <p style={{color:"#9ca3af",fontSize:13,marginBottom:18}}>Tài khoản: <strong style={{color:"#fff"}}>VinFast Official</strong></p>
                <Row style={{gap:11,justifyContent:"center"}}>{[{l:"Nhóm",v:"12"},{l:"Leads hôm nay",v:"34"},{l:"Agents",v:"3"}].map(s=><div key={s.l} style={{background:"#2a2f45",borderRadius:9,padding:"9px 16px",textAlign:"center"}}><p style={{color:"#fff",fontWeight:700,fontSize:17}}>{s.v}</p><p style={{color:"#9ca3af",fontSize:10}}>{s.l}</p></div>)}</Row>
              </div>
            )}
          </div>
        </div>
      </div>
    );

    if(tab==="inbox") return(
      <div style={{display:"flex",gap:14,height:420}}>
        <div style={{width:228,background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflowY:"auto",flexShrink:0}}>
          <div style={{padding:"11px 13px",borderBottom:"1px solid #2a2f45"}}><p style={{color:"#e5e7eb",fontWeight:500,fontSize:13}}>Tất cả hội thoại</p></div>
          {THREADS.map(t=><div key={t.id} onClick={()=>setAthr(t)} style={{padding:"11px 13px",borderBottom:"1px solid #1a1f35",cursor:"pointer",background:athr.id===t.id?"#2a2f45":"transparent"}}>
            <Row style={{gap:7,marginBottom:4}}><Av t={t.lead[0]} sz={24}/><div style={{flex:1,minWidth:0}}><Row style={{justifyContent:"space-between"}}><p style={{color:"#e5e7eb",fontSize:12,fontWeight:500,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap",maxWidth:120}}>{t.lead}</p>{t.unread>0&&<span style={{background:"#4f46e5",color:"#fff",fontSize:10,fontWeight:700,width:15,height:15,borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center",flexShrink:0}}>{t.unread}</span>}</Row><p style={{color:"#6b7280",fontSize:10}}>{t.agent}</p></div></Row>
            <p style={{color:"#9ca3af",fontSize:11,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap",marginBottom:5}}>{t.last}</p>
            <Row style={{justifyContent:"space-between"}}><Bdg label={t.status}/><span style={{color:"#4b5563",fontSize:11}}>{t.time}</span></Row>
          </div>)}
        </div>
        <div style={{flex:1,background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,display:"flex",flexDirection:"column"}}>
          <Row style={{gap:10,padding:"11px 15px",borderBottom:"1px solid #2a2f45"}}><Av t={athr.lead[0]} sz={30}/><div><p style={{color:"#e5e7eb",fontWeight:500,fontSize:13}}>{athr.lead}</p><p style={{color:"#6b7280",fontSize:11}}>via {athr.agent}</p></div><div style={{marginLeft:8}}><Bdg label={athr.status}/></div></Row>
          <div style={{flex:1,overflowY:"auto",padding:14,display:"flex",flexDirection:"column",gap:10}}>
            {[{f:"lead",t:"Anh ơi xe VF8 còn hàng không ạ?",time:"14:20"},{f:"agent",t:"Dạ chào anh! Còn màu đen và trắng ạ. Anh test drive cuối tuần không?",time:"14:21"},{f:"lead",t:"Màu đen còn không?",time:"14:22"},{f:"agent",t:"Dạ còn ạ! Mình đặt lịch cho anh nhé?",time:"14:23"}].map((m,i)=><div key={i} style={{display:"flex",justifyContent:m.f==="agent"?"flex-end":"flex-start"}}>
              <div style={{maxWidth:"72%",padding:"9px 13px",borderRadius:13,background:m.f==="agent"?"#4f46e5":"#2a2f45",color:"#fff"}}>
                {m.f==="agent"&&<p style={{color:"#a5b4fc",fontSize:10,marginBottom:3}}>{athr.agent}</p>}
                <p style={{fontSize:13}}>{m.t}</p>
                <p style={{fontSize:10,color:m.f==="agent"?"#a5b4fc":"#6b7280",marginTop:3,textAlign:"right"}}>{m.time}</p>
              </div>
            </div>)}
          </div>
          <Row style={{gap:9,padding:"11px 14px",borderTop:"1px solid #2a2f45"}}>
            <input placeholder="Nhập tin nhắn..." style={{...inp,flex:1,padding:"8px 12px"}}/>
            <button style={PB({padding:"8px 11px"})}><Send size={14} color="#fff"/></button>
          </Row>
        </div>
      </div>
    );

    if(tab==="posting") return(
      <div style={{display:"flex",flexDirection:"column",gap:12}}>
        <Row style={{gap:8}}>
          {["Tất cả","Live","Đã kết thúc"].map(f=><button key={f} style={SB({padding:"6px 13px",fontSize:12})}>{f}</button>)}
          <button style={{...PB({padding:"6px 13px",fontSize:12}),marginLeft:"auto",display:"flex",alignItems:"center",gap:5}}><Plus size={13}/>Tạo bài viết</button>
        </Row>
        {POSTS.map(p=><div key={p.id} style={card()}>
          <Row style={{gap:8,marginBottom:8}}><span style={{background:"#2a2f45",color:"#d1d5db",padding:"2px 8px",borderRadius:5,fontSize:10}}>{p.group}</span><span style={{color:"#6b7280",fontSize:11}}>{p.time}</span><Bdg label={p.status}/></Row>
          <p style={{color:"#d1d5db",fontSize:13,lineHeight:1.6}}>{p.content}</p>
          <Row style={{gap:20,marginTop:12,paddingTop:12,borderTop:"1px solid #2a2f45"}}>
            {[{I:ThumbsUp,v:p.likes,l:"Likes"},{I:MessageCircle,v:p.comments,l:"Comments"},{I:Share2,v:p.shares,l:"Shares"}].map(s=><Row key={s.l} style={{gap:5,color:"#9ca3af"}}><s.I size={12}/><span style={{color:"#e5e7eb",fontWeight:500,fontSize:12}}>{s.v}</span><span style={{fontSize:11}}>{s.l}</span></Row>)}
            <button style={{marginLeft:"auto",background:"none",border:"none",color:"#818cf8",fontSize:11,cursor:"pointer",display:"flex",alignItems:"center",gap:4}}><Eye size={11}/>Xem bài</button>
          </Row>
        </div>)}
      </div>
    );

    if(tab==="commenting") return(
      <div style={{background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflow:"hidden"}}>
        <div style={{display:"grid",gridTemplateColumns:"110px 120px 150px 1fr 70px",padding:"9px 14px",background:"#151824",borderBottom:"1px solid #2a2f45"}}>
          {["Agent","Lead","Bài viết","Nội dung","Giờ"].map(h=><p key={h} style={{color:"#6b7280",fontSize:11,fontWeight:500}}>{h}</p>)}
        </div>
        {CMTS.map(c=><div key={c.id} style={{display:"grid",gridTemplateColumns:"110px 120px 150px 1fr 70px",padding:"11px 14px",borderBottom:"1px solid #1a1f35",alignItems:"center"}}>
          <span style={{background:"#312e8133",color:"#a5b4fc",border:"1px solid #4f46e544",padding:"2px 8px",borderRadius:6,fontSize:10,width:"fit-content"}}>{c.agent}</span>
          <p style={{color:"#d1d5db",fontSize:12,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>{c.lead}</p>
          <p style={{color:"#6b7280",fontSize:11,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>{c.post}</p>
          <p style={{color:"#d1d5db",fontSize:12,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>{c.comment}</p>
          <p style={{color:"#6b7280",fontSize:11}}>{c.time}</p>
        </div>)}
      </div>
    );

    if(tab==="leaderboard") return(
      <div style={{display:"flex",flexDirection:"column",gap:12}}>
        <Row style={{justifyContent:"space-between"}}>
          <p style={{color:"#e5e7eb",fontWeight:600,fontSize:14}}>KPI Tháng 4 / 2025</p>
          <button onClick={()=>setShowCfg(!showCfg)} style={{display:"flex",alignItems:"center",gap:6,...SB({padding:"6px 13px",fontSize:12})}}><Settings size={12}/>Cấu hình KPI</button>
        </Row>
        {showCfg&&<div style={card({border:"1px solid #92400e55",background:"#1c1a0e"})}>
          <p style={{color:"#fbbf24",fontWeight:500,fontSize:12,marginBottom:14}}>Cấu hình điểm & thưởng phạt — Admin only (không cần coder)</p>
          <div style={{display:"grid",gridTemplateColumns:"repeat(3,1fr)",gap:11}}>
            {[{l:"Điểm / Conversation",k:"conv"},{l:"Điểm / Converted",k:"conv2"},{l:"Điểm / Comment",k:"cmt"},{l:"Ngưỡng thưởng (điểm)",k:"bonus"},{l:"Tiền thưởng (VNĐ)",k:"bonusAmt"},{l:"Ngưỡng phạt (điểm)",k:"pen"}].map(f=><div key={f.k}><Lbl t={f.l}/><input type="number" value={cfg[f.k]} onChange={e=>setCfg(p=>({...p,[f.k]:+e.target.value}))} style={{...inp,padding:"7px 11px"}}/></div>)}
          </div>
        </div>}
        {scored.map((s,i)=><div key={s.id} style={card({border:`1px solid ${i===0?"#d9780655":"#2a2f45"}`,display:"flex",alignItems:"center",gap:12})}>
          <div style={{width:30,height:30,borderRadius:"50%",background:["#d97706","#9ca3af","#b45309","#374151"][i]||"#374151",display:"flex",alignItems:"center",justifyContent:"center",color:"#fff",fontWeight:700,fontSize:13,flexShrink:0}}>{i+1}</div>
          <Av t={s.name[0]} bg={i===0?"#d97706":"#4f46e5"} sz={34}/>
          <div style={{flex:1}}>
            <Row style={{gap:7,marginBottom:4}}><p style={{color:"#e5e7eb",fontWeight:600,fontSize:13}}>{s.name}</p>{i===0&&<Trophy size={13} color="#fbbf24"/>}<span style={{color:"#6b7280",fontSize:11}}>{s.role}</span></Row>
            <Row style={{gap:14}}>{[{l:"Convs",v:s.convs,c:"#fff"},{l:"Converted",v:s.converted,c:"#4ade80"},{l:"Comments",v:s.cmts||"—",c:"#fff"}].map(x=><span key={x.l} style={{color:"#9ca3af",fontSize:11}}>{x.l}: <strong style={{color:x.c}}>{x.v}</strong></span>)}</Row>
          </div>
          <div style={{textAlign:"right"}}>
            <p style={{fontSize:20,fontWeight:800,color:i===0?"#fbbf24":"#e5e7eb"}}>{s.pts.toLocaleString()}</p>
            <p style={{color:"#6b7280",fontSize:11}}>điểm</p>
            {s.pts>=cfg.bonus&&<p style={{color:"#4ade80",fontSize:11}}>+{cfg.bonusAmt.toLocaleString()}đ</p>}
            {s.pts<cfg.pen&&<p style={{color:"#f87171",fontSize:11}}>-{cfg.penAmt?.toLocaleString()||cfg.penaltyAmt?.toLocaleString()||0}đ</p>}
          </div>
        </div>)}
      </div>
    );

    if(tab==="data") return(
      <div style={{display:"flex",flexDirection:"column",gap:14}}>
        <div onDragOver={e=>{e.preventDefault();setDrag(true);}} onDragLeave={()=>setDrag(false)} onDrop={()=>setDrag(false)} style={{border:`2px dashed ${drag?"#6366f1":"#374151"}`,borderRadius:14,padding:"36px 24px",textAlign:"center",background:drag?"#312e8111":"transparent"}}>
          <Upload size={28} color="#6b7280" style={{margin:"0 auto 11px",display:"block"}}/>
          <p style={{color:"#e5e7eb",fontWeight:500,fontSize:14,marginBottom:5}}>Upload dữ liệu kinh doanh</p>
          <p style={{color:"#9ca3af",fontSize:12,marginBottom:18}}>Kéo thả hoặc click · PDF, Excel, TXT, CSV</p>
          <button style={SB({fontSize:12,padding:"7px 16px"})}>Chọn file</button>
        </div>
        <div style={{background:"#1e2130",border:"1px solid #2a2f45",borderRadius:12,overflow:"hidden"}}>
          <Row style={{justifyContent:"space-between",padding:"11px 14px",borderBottom:"1px solid #2a2f45"}}><p style={{color:"#e5e7eb",fontWeight:500,fontSize:13}}>Files đã upload</p><span style={{color:"#6b7280",fontSize:11}}>{FILES.length} files · AI indexed</span></Row>
          {FILES.map(f=><Row key={f.id} style={{gap:10,padding:"11px 14px",borderBottom:"1px solid #1a1f35"}}>
            <div style={{width:32,height:32,background:"#312e8133",border:"1px solid #4f46e544",borderRadius:8,display:"flex",alignItems:"center",justifyContent:"center"}}><Database size={13} color="#818cf8"/></div>
            <div style={{flex:1}}><p style={{color:"#d1d5db",fontSize:12,fontWeight:500}}>{f.name}</p><p style={{color:"#6b7280",fontSize:11}}>{f.size} · {f.date}</p></div>
            <Bdg label="Indexed"/>
            <button style={{background:"none",border:"none",cursor:"pointer",color:"#6b7280"}}><X size={13}/></button>
          </Row>)}
        </div>
        <div style={{background:"#312e8111",border:"1px solid #4f46e544",borderRadius:11,padding:"13px 15px",display:"flex",alignItems:"flex-start",gap:10}}>
          <Zap size={15} color="#818cf8" style={{marginTop:2,flexShrink:0}}/>
          <div><p style={{color:"#a5b4fc",fontWeight:500,fontSize:13,marginBottom:3}}>AI Context đang hoạt động</p><p style={{color:"#818cf866",fontSize:11}}>{FILES.length} files đã được index, AI agents đang sử dụng dữ liệu theo tệp riêng của tổ chức bạn.</p></div>
        </div>
      </div>
    );

    if(tab==="settings") return <SettingsPage org={org}/>;
    return null;
  };

  return(
    <div style={{display:"flex",height:630,background:"#0d101a",fontFamily:"system-ui,sans-serif",borderRadius:16,overflow:"hidden",border:"1px solid #1e2130"}}>
      <style>{`@keyframes spin{to{transform:rotate(360deg)}}`}</style>
      {/* SIDEBAR */}
      <div style={{width:200,background:"#111520",borderRight:"1px solid #1e2130",display:"flex",flexDirection:"column",flexShrink:0}}>
        <div style={{padding:"14px 13px",borderBottom:"1px solid #1e2130"}}>
          <Row style={{gap:9}}>
            <div style={{width:30,height:30,background:"#4f46e5",borderRadius:8,display:"flex",alignItems:"center",justifyContent:"center"}}><Zap size={14} color="#fff"/></div>
            <div><p style={{color:"#e5e7eb",fontWeight:800,fontSize:13,lineHeight:1}}>AutoFlow</p><p style={{color:"#6b7280",fontSize:10}}>{isAdmin?"Admin":"Staff"} Portal</p></div>
          </Row>
        </div>
        {/* Org switcher (admin only) */}
        {isAdmin&&<div style={{padding:"9px 10px",borderBottom:"1px solid #1e2130",position:"relative"}}>
          <button onClick={()=>setOrgMenu(!orgMenu)} style={{width:"100%",display:"flex",alignItems:"center",gap:7,padding:"7px 9px",background:"#1e2130",border:"1px solid #2a2f45",borderRadius:8,cursor:"pointer"}}>
            <div style={{width:24,height:24,borderRadius:6,background:org.color,display:"flex",alignItems:"center",justifyContent:"center",color:"#fff",fontSize:9,fontWeight:700,flexShrink:0}}>{org.abbr}</div>
            <div style={{flex:1,textAlign:"left",minWidth:0}}><p style={{color:"#e5e7eb",fontSize:11,fontWeight:500,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>{org.name}</p><p style={{color:"#6b7280",fontSize:9}}>{org.plan}</p></div>
            <ChevronDown size={11} color="#6b7280"/>
          </button>
          {orgMenu&&<div style={{position:"absolute",left:10,right:10,top:"calc(100% - 2px)",background:"#1e2130",border:"1px solid #2a2f45",borderRadius:10,zIndex:99,overflow:"hidden",boxShadow:"0 8px 24px #00000066"}}>
            {ORGS.map(o=><button key={o.id} onClick={()=>{setOrg(o);setOrgMenu(false);}} style={{width:"100%",display:"flex",alignItems:"center",gap:7,padding:"9px 11px",background:"none",border:"none",cursor:"pointer",textAlign:"left"}}>
              <div style={{width:20,height:20,borderRadius:5,background:o.color,display:"flex",alignItems:"center",justifyContent:"center",color:"#fff",fontSize:8,fontWeight:700,flexShrink:0}}>{o.abbr}</div>
              <div style={{flex:1}}><p style={{color:"#e5e7eb",fontSize:11}}>{o.name}</p><p style={{color:"#6b7280",fontSize:9}}>{o.plan}</p></div>
              {org.id===o.id&&<Check size={11} color="#818cf8"/>}
            </button>)}
            <div style={{padding:"5px 9px",borderTop:"1px solid #2a2f45"}}><button style={{display:"flex",alignItems:"center",gap:5,background:"none",border:"none",color:"#818cf8",fontSize:11,cursor:"pointer",padding:"3px 3px"}}><Plus size={11}/>Thêm org</button></div>
          </div>}
        </div>}
        {/* Nav */}
        <nav style={{flex:1,padding:"9px 9px",display:"flex",flexDirection:"column",gap:1}}>
          {TABS.map(({id,l,I,b})=><button key={id} onClick={()=>setTab(id)} style={{width:"100%",display:"flex",alignItems:"center",gap:9,padding:"7px 11px",borderRadius:8,border:"none",cursor:"pointer",background:tab===id?"#4f46e5":"transparent",color:tab===id?"#fff":"#9ca3af",fontSize:12,fontWeight:tab===id?600:400}}>
            <I size={13}/><span style={{flex:1,textAlign:"left"}}>{l}</span>
            {b&&tab!==id&&<span style={{background:"#ef4444",color:"#fff",fontSize:9,fontWeight:700,width:14,height:14,borderRadius:"50%",display:"flex",alignItems:"center",justifyContent:"center"}}>{b}</span>}
          </button>)}
        </nav>
        {/* Bottom */}
        <div style={{padding:"9px 9px",borderTop:"1px solid #1e2130"}}>
          {isAdmin&&<button onClick={()=>setTab("settings")} style={{width:"100%",display:"flex",alignItems:"center",gap:9,padding:"7px 11px",borderRadius:8,border:"none",cursor:"pointer",background:tab==="settings"?"#4f46e5":"transparent",color:tab==="settings"?"#fff":"#9ca3af",fontSize:12,marginBottom:8}}><Settings size={13}/>Settings</button>}
          <Row style={{gap:8,padding:"0 5px"}}>
            <Av t={isAdmin?"AD":"ST"} bg={isAdmin?"#7c3aed":"#0ea5e9"} sz={28}/>
            <div style={{flex:1,minWidth:0}}>
              <p style={{color:"#e5e7eb",fontSize:11,fontWeight:500}}>{isAdmin?"Admin":"Nguyễn Hữu Đức"}</p>
              <button onClick={goLanding} style={{background:"none",border:"none",color:"#6b7280",fontSize:10,cursor:"pointer",padding:0}}>← Thoát</button>
            </div>
          </Row>
        </div>
      </div>
      {/* MAIN */}
      <div style={{flex:1,display:"flex",flexDirection:"column",overflow:"hidden"}}>
        <Row style={{gap:14,padding:"11px 18px",background:"#111520",borderBottom:"1px solid #1e2130"}}>
          <div><p style={{color:"#f9fafb",fontWeight:700,fontSize:14}}>{tabLabel}</p><p style={{color:"#6b7280",fontSize:11}}>{org.name}</p></div>
          <Row style={{marginLeft:"auto",gap:11}}>
            {!isAdmin&&<span style={{background:"#312e8122",border:"1px solid #4f46e544",color:"#a5b4fc",fontSize:11,padding:"4px 10px",borderRadius:7}}>Staff View</span>}
            <Row style={{gap:5,background:"#052e1622",border:"1px solid #16a34a44",borderRadius:7,padding:"4px 9px"}}><div style={{width:6,height:6,background:"#4ade80",borderRadius:"50%"}}/><span style={{color:"#4ade80",fontSize:11}}>3 agents online</span></Row>
            <button style={{background:"none",border:"none",cursor:"pointer",position:"relative",padding:5}}><Bell size={15} color="#9ca3af"/><span style={{position:"absolute",top:3,right:3,width:6,height:6,background:"#ef4444",borderRadius:"50%",border:"1.5px solid #111520"}}/></button>
          </Row>
        </Row>
        <div style={{flex:1,overflowY:"auto",padding:18}}><Content/></div>
      </div>
    </div>
  );
}

// ═════════════════════════════════════════════════════════════════
// ROOT
// ═════════════════════════════════════════════════════════════════
export default function Root(){
  const [view,setView]=useState("landing");
  const [authMode,setAuthMode]=useState("login");
  const [role,setRole]=useState("admin");
  const goAuth=(m="login")=>{setAuthMode(m);setView("auth");};
  const goApp=(r="admin")=>{setRole(r);setView("app");};
  if(view==="landing") return <Landing onLogin={()=>goAuth("login")} onRegister={()=>goAuth("register")} onAdmin={()=>setView("superadmin")}/>;
  if(view==="auth") return <Auth mode={authMode} setMode={m=>{if(m==="app")goApp("admin");else setAuthMode(m);}} onSuccess={(r)=>goApp(r)} goBack={()=>setView("landing")}/>;
  if(view==="superadmin") return <SuperAdmin goBack={()=>setView("landing")}/>;
  return <MainApp role={role} goLanding={()=>setView("landing")}/>;
}
