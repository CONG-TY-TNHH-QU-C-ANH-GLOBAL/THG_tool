import type { Lead, Thread, Post, Comment, StaffMember, FileRecord, Organization, OrgSummary, KPIConfig } from '../types';

export const MOCK_LEADS: Lead[] = [
  {id:1,name:"Nguyễn Văn An",status:"Hot",group:"Ô tô VN",agent:"Agent_01",last:"2g",score:92,phone:"0901 234 567"},
  {id:2,name:"Trần Thị Bích",status:"Warm",group:"Xe Miền Nam",agent:"Agent_02",last:"5g",score:75,phone:"0912 345 678"},
  {id:3,name:"Lê Hoàng Minh",status:"Cold",group:"Ô tô VN",agent:"Agent_01",last:"1 ngày",score:45,phone:"0923 456 789"},
  {id:4,name:"Phạm Thu Hà",status:"Hot",group:"Nội thất",agent:"Agent_03",last:"30p",score:88,phone:"0934 567 890"},
  {id:5,name:"Vũ Đình Tùng",status:"Warm",group:"Xe Miền Bắc",agent:"Agent_02",last:"3g",score:67,phone:"0945 678 901"},
  {id:6,name:"Đặng Ngọc Lan",status:"Hot",group:"Ô tô VN",agent:"Agent_01",last:"1g",score:95,phone:"0956 789 012"},
];

export const MOCK_THREADS: Thread[] = [
  {id:1,lead:"Nguyễn Văn An",agent:"Agent_01",last:"Anh ơi xe còn hàng không ạ?",time:"2p",status:"Active",unread:2},
  {id:2,lead:"Phạm Thu Hà",agent:"Agent_03",last:"Em muốn đặt cọc giữ xe",time:"15p",status:"Converted",unread:0},
  {id:3,lead:"Đặng Ngọc Lan",agent:"Agent_01",last:"Giá tốt nhất là bao nhiêu?",time:"1g",status:"Active",unread:5},
  {id:4,lead:"Vũ Đình Tùng",agent:"Agent_02",last:"Cho em xin báo giá chi tiết",time:"3g",status:"Pending",unread:1},
];

export const MOCK_POSTS: Post[] = [
  {id:1,group:"Ô tô VN",content:"🔥 VinFast VF8 — Ưu đãi tháng 5! Giảm 50 triệu, hỗ trợ vay 0%...",time:"10g",likes:234,comments:67,shares:45,status:"Live"},
  {id:2,group:"Xe Miền Nam",content:"🎁 Mua xe VinFast nhận ngay phụ kiện chính hãng 20 triệu đồng...",time:"1 ngày",likes:189,comments:43,shares:28,status:"Live"},
  {id:3,group:"Xe Miền Bắc",content:"⚡ VF9 Premium — Xe điện sang trọng nhất thị trường 2025...",time:"2 ngày",likes:456,comments:112,shares:89,status:"Ended"},
];

export const MOCK_COMMENTS: Comment[] = [
  {id:1,agent:"Agent_01",lead:"Nguyễn Văn An",post:"VF8 ưu đãi",comment:"Dạ anh ơi, còn màu đen và trắng ạ, mình inbox cho anh nhé!",time:"1g"},
  {id:2,agent:"Agent_02",lead:"Trần Thị Bích",post:"Mua xe nhận quà",comment:"Bạn Bích ơi mình tư vấn kỹ hơn qua inbox ạ!",time:"2g"},
  {id:3,agent:"Agent_01",lead:"Đặng Ngọc Lan",post:"VF8 ưu đãi",comment:"Chị Lan ơi, giá tốt nhất mình nhắn inbox ạ",time:"3g"},
];

export const MOCK_STAFF: StaffMember[] = [
  {id:1,name:"Nguyễn Hữu Đức",email:"duc@vinfast.vn",role:"Senior Sales",status:"Active",joined:"01/01/2025",convs:45,converted:12,cmts:38},
  {id:2,name:"Trần Minh Châu",email:"chau@vinfast.vn",role:"Sales",status:"Active",joined:"05/01/2025",convs:38,converted:9,cmts:21},
  {id:3,name:"Lê Thị Thanh",email:"thanh@vinfast.vn",role:"Sales",status:"Active",joined:"10/01/2025",convs:31,converted:11,cmts:44},
  {id:4,name:"Hoàng Văn Bình",email:"binh@vinfast.vn",role:"Junior Sales",status:"Suspended",joined:"15/01/2025",convs:22,converted:5,cmts:9},
];

export const MOCK_FILES: FileRecord[] = [
  {id:1,name:"product_catalog_2025.pdf",size:"2.4 MB",date:"20/04"},
  {id:2,name:"price_list_Q2.xlsx",size:"1.1 MB",date:"18/04"},
  {id:3,name:"faq_customers.txt",size:"156 KB",date:"15/04"},
];

export const MOCK_ORGS: Organization[] = [
  {id:1,name:"VinFast Sản Xuất",abbr:"VF",plan:"Enterprise",color:"#4f46e5"},
  {id:2,name:"Thế Giới Di Động",abbr:"TG",plan:"Pro",color:"#0ea5e9"},
  {id:3,name:"FPT Software",abbr:"FP",plan:"Pro",color:"#10b981"},
];

export const MOCK_ORGS_SUMMARY: OrgSummary[] = [
  {id:1,name:"VinFast Sản Xuất",plan:"Enterprise",users:12,status:"Active",joined:"01/03/2025",rev:"₫6.9M"},
  {id:2,name:"Thế Giới Di Động",plan:"Pro",users:8,status:"Active",joined:"10/03/2025",rev:"₫2.9M"},
  {id:3,name:"FPT Software",plan:"Pro",users:5,status:"Active",joined:"15/03/2025",rev:"₫2.9M"},
  {id:4,name:"Vinamilk Group",plan:"Starter",users:3,status:"Active",joined:"20/03/2025",rev:"₫990K"},
  {id:5,name:"An Khang Pharmacy",plan:"Pro",users:6,status:"Suspended",joined:"01/04/2025",rev:"₫0"},
];

export const DEFAULT_KPI_CONFIG: KPIConfig = {
  conv: 10, conv2: 50, cmt: 2,
  bonus: 1000, bonusAmt: 500000,
  pen: 300, penAmt: 100000,
};
