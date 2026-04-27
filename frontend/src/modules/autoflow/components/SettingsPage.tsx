import { useState } from 'react';
import type { Organization } from '../types';
import { Avatar, Badge, Row } from './ui';
import { theme, cardStyle, primaryBtn, secondaryBtn } from '../constants/styles';
import { useStaff } from '../hooks/useStaff';
import { Palette, Shield, Users, Zap, CreditCard, UserPlus, Check, X, Upload } from 'lucide-react';

interface SettingsPageProps { org: Organization; orgId: string; isAdmin: boolean; }

type SettingsTab = 'brand' | 'security' | 'staff' | 'agents' | 'billing';

const inp = { background: '#2a2f45', border: '1px solid #374151', borderRadius: 9, padding: '10px 14px', color: '#fff', fontSize: 13, outline: 'none', width: '100%', boxSizing: 'border-box' as const };
const Lbl = ({ t }: { t: string }) => <p style={{ color: theme.textFaint, fontSize: 12, marginBottom: 5 }}>{t}</p>;

const TABS: { id: SettingsTab; l: string; I: React.ComponentType<{ size?: number | string }> }[] = [
  { id: 'brand', l: 'Thương hiệu', I: Palette },
  { id: 'security', l: 'Bảo mật', I: Shield },
  { id: 'staff', l: 'Nhân viên', I: Users },
  { id: 'agents', l: 'AI Agents', I: Zap },
  { id: 'billing', l: 'Thanh toán', I: CreditCard },
];

export default function SettingsPage({ org, orgId, isAdmin }: SettingsPageProps) {
  const [activeTab, setActiveTab] = useState<SettingsTab>('brand');
  const { staff, add, toggleStatus, remove } = useStaff(orgId);
  const [showAdd, setShowAdd] = useState(false);
  const [newStaff, setNewStaff] = useState({ name: '', email: '', role: 'Sales' });
  const [pwOk, setPwOk] = useState(false);
  const [color, setColor] = useState(org.color || theme.primary);
  const [abbr, setAbbr] = useState(org.abbr || 'ORG');

  const handleAdd = async () => {
    if (!newStaff.name || !newStaff.email) return;
    await add(newStaff);
    setNewStaff({ name: '', email: '', role: 'Sales' });
    setShowAdd(false);
  };

  return (
    <div>
      {/* Tab bar */}
      <div style={{ display: 'flex', gap: 6, marginBottom: 22, flexWrap: 'wrap' }}>
        {TABS.map(({ id, l, I }) => (
          <button key={id} onClick={() => setActiveTab(id)} style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '7px 13px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 12,
            background: activeTab === id ? theme.primary : theme.surface,
            color: activeTab === id ? '#fff' : theme.textMuted,
          }}>
            <I size={12} />{l}
          </button>
        ))}
      </div>

      {/* BRANDING */}
      {activeTab === 'brand' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 18 }}>Nhận diện thương hiệu</p>
            <div style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: 22, alignItems: 'start' }}>
              <div style={{ textAlign: 'center' }}>
                <div style={{ width: 80, height: 80, background: color, borderRadius: 18, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 28, fontWeight: 900, marginBottom: 10, border: `3px solid ${color}44` }}>
                  {abbr}
                </div>
                <p style={{ color: theme.textFaint, fontSize: 11 }}>Avatar / Logo</p>
                <button style={{ ...secondaryBtn({ padding: '4px 10px', fontSize: 11 }), marginTop: 6 }}>Upload</button>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <div><Lbl t="Tên tổ chức" /><input style={inp} defaultValue={org.name} /></div>
                <div>
                  <Lbl t="Viết tắt (2–3 ký tự)" />
                  <input style={inp} value={abbr} onChange={e => setAbbr(e.target.value.slice(0, 3).toUpperCase())} placeholder="VF" />
                </div>
                <div>
                  <Lbl t="Màu thương hiệu" />
                  <Row style={{ gap: 8, alignItems: 'center' }}>
                    <input type="color" value={color} onChange={e => setColor(e.target.value)} style={{ width: 38, height: 34, border: 'none', borderRadius: 8, cursor: 'pointer', background: 'none' }} />
                    <input style={{ ...inp, flex: 1 }} value={color} onChange={e => setColor(e.target.value)} />
                  </Row>
                </div>
                <div><Lbl t="Ngành" /><select style={inp}><option>Sản xuất</option><option>Bán lẻ</option><option>Công nghệ</option><option>Bất động sản</option></select></div>
              </div>
            </div>
          </div>
          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 14 }}>Upload logo đầy đủ</p>
            <div style={{ border: '2px dashed #374151', borderRadius: 12, padding: 30, textAlign: 'center' }}>
              <Upload size={26} color={theme.textFaint} style={{ margin: '0 auto 10px', display: 'block' }} />
              <p style={{ color: theme.textMuted, fontSize: 12, marginBottom: 12 }}>PNG, SVG khuyến nghị · Nền trong suốt</p>
              <button style={secondaryBtn({ fontSize: 12, padding: '6px 14px' }) as React.CSSProperties}>Chọn file</button>
            </div>
          </div>
          <button style={{ ...primaryBtn({ padding: '10px 24px' }), alignSelf: 'flex-end' } as React.CSSProperties}>Lưu thay đổi</button>
        </div>
      )}

      {/* SECURITY */}
      {activeTab === 'security' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14, maxWidth: 480 }}>
          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 18 }}>Đổi mật khẩu</p>
            {!pwOk ? (
              <>
                <Lbl t="Mật khẩu hiện tại" /><input type="password" placeholder="••••••••" style={{ ...inp, marginBottom: 13 }} />
                <Lbl t="Mật khẩu mới" /><input type="password" placeholder="Tối thiểu 8 ký tự" style={{ ...inp, marginBottom: 13 }} />
                <Lbl t="Xác nhận mật khẩu mới" /><input type="password" placeholder="Nhập lại" style={{ ...inp, marginBottom: 18 }} />
                <button onClick={() => setPwOk(true)} style={primaryBtn({ padding: '10px 22px' }) as React.CSSProperties}>Cập nhật mật khẩu</button>
              </>
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 14, background: '#16a34a22', border: '1px solid #16a34a44', borderRadius: 10 }}>
                <Check size={18} color="#4ade80" />
                <div>
                  <p style={{ color: '#4ade80', fontWeight: 500, fontSize: 13 }}>Mật khẩu đã được cập nhật!</p>
                  <button onClick={() => setPwOk(false)} style={{ background: 'none', border: 'none', color: theme.textMuted, fontSize: 11, cursor: 'pointer', padding: 0, marginTop: 3 }}>Đổi lại</button>
                </div>
              </div>
            )}
          </div>
          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 10 }}>Phiên đăng nhập</p>
            {[
              { d: 'Chrome · Ho Chi Minh City', t: 'Đang hoạt động', active: true },
              { d: 'Safari Mobile · Hà Nội', t: '3 giờ trước', active: false },
            ].map((s, i) => (
              <div key={i} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '9px 0', borderBottom: `1px solid ${theme.border}` }}>
                <div>
                  <p style={{ color: theme.text, fontSize: 13 }}>{s.d}</p>
                  <p style={{ color: s.active ? '#4ade80' : theme.textFaint, fontSize: 11 }}>{s.t}</p>
                </div>
                {!s.active && (
                  <button style={{ background: 'none', border: `1px solid ${theme.border}`, borderRadius: 6, color: '#f87171', fontSize: 11, padding: '4px 10px', cursor: 'pointer' }}>Thu hồi</button>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* STAFF */}
      {activeTab === 'staff' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <p style={{ color: theme.textMuted, fontSize: 13 }}>{staff.length} nhân viên</p>
            {isAdmin && (
              <button onClick={() => setShowAdd(!showAdd)} style={{ ...primaryBtn({ padding: '8px 15px', fontSize: 12 }), display: 'flex', alignItems: 'center', gap: 6 } as React.CSSProperties}>
                <UserPlus size={13} />Thêm nhân viên
              </button>
            )}
          </div>

          {showAdd && (
            <div style={{ ...cardStyle(), border: `1px solid ${theme.primary}44` }}>
              <p style={{ color: theme.primaryPale, fontWeight: 500, fontSize: 13, marginBottom: 14 }}>Thêm nhân viên mới</p>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr auto', gap: 10, alignItems: 'end' }}>
                <div><Lbl t="Họ tên" /><input style={inp} value={newStaff.name} onChange={e => setNewStaff(p => ({ ...p, name: e.target.value }))} placeholder="Nguyễn Văn A" /></div>
                <div><Lbl t="Email" /><input style={inp} value={newStaff.email} onChange={e => setNewStaff(p => ({ ...p, email: e.target.value }))} placeholder="nva@company.vn" /></div>
                <div><Lbl t="Vai trò" /><select style={inp} value={newStaff.role} onChange={e => setNewStaff(p => ({ ...p, role: e.target.value }))}>
                  <option>Sales</option><option>Senior Sales</option><option>Team Lead</option>
                </select></div>
                <button onClick={handleAdd} style={primaryBtn({ padding: '10px 14px' }) as React.CSSProperties}>Thêm</button>
              </div>
              <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 9 }}>Nhân viên nhận email mời, tự đặt mật khẩu lần đầu đăng nhập.</p>
            </div>
          )}

          <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  {['Nhân viên', 'Email', 'Vai trò', 'Convs', 'Chốt', 'Tham gia', 'Status', ''].map(h => (
                    <th key={h} style={{ padding: '10px 13px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {staff.map(s => (
                  <tr key={s.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                    <td style={{ padding: '10px 13px' }}>
                      <Row style={{ gap: 8 }}>
                        <Avatar text={s.name[0]} size={26} />
                        <span style={{ color: theme.text, fontWeight: 500 }}>{s.name}</span>
                      </Row>
                    </td>
                    <td style={{ padding: '10px 13px', color: theme.textMuted }}>{s.email}</td>
                    <td style={{ padding: '10px 13px', color: '#d1d5db' }}>{s.role}</td>
                    <td style={{ padding: '10px 13px', color: '#d1d5db' }}>{s.convs}</td>
                    <td style={{ padding: '10px 13px', color: '#4ade80' }}>{s.converted}</td>
                    <td style={{ padding: '10px 13px', color: theme.textFaint }}>{s.joined}</td>
                    <td style={{ padding: '10px 13px' }}><Badge label={s.status} /></td>
                    <td style={{ padding: '10px 13px' }}>
                      <Row style={{ gap: 6 }}>
                        {isAdmin && (
                          <>
                            <button onClick={() => toggleStatus(s.id)} style={{ background: 'none', border: `1px solid ${theme.border}`, borderRadius: 6, color: theme.textMuted, fontSize: 10, padding: '3px 8px', cursor: 'pointer' }}>
                              {s.status === 'Active' ? 'Tạm dừng' : 'Kích hoạt'}
                            </button>
                            <button onClick={() => remove(s.id)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: theme.textFaint }}>
                              <X size={13} />
                            </button>
                          </>
                        )}
                      </Row>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* AGENTS */}
      {activeTab === 'agents' && (
        <div style={cardStyle()}>
          <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 14 }}>AI Agents đang chạy</p>
          {['Agent_01', 'Agent_02', 'Agent_03'].map(a => (
            <div key={a} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '11px 0', borderBottom: `1px solid ${theme.border}` }}>
              <Row style={{ gap: 10 }}>
                <div style={{ width: 8, height: 8, background: '#4ade80', borderRadius: '50%' }} />
                <div>
                  <p style={{ color: '#d1d5db', fontSize: 13, fontWeight: 500 }}>{a}</p>
                  <p style={{ color: theme.textFaint, fontSize: 11 }}>GPT-4o · 3 nhóm · 120 messages/ngày</p>
                </div>
              </Row>
              <button style={{ background: 'none', border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.primaryLight, fontSize: 12, padding: '6px 13px', cursor: 'pointer' }}>Cấu hình</button>
            </div>
          ))}
        </div>
      )}

      {/* BILLING */}
      {activeTab === 'billing' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={{ ...cardStyle(), border: `1px solid ${theme.primary}44` }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 }}>
              <div>
                <p style={{ color: theme.textMuted, fontSize: 11, marginBottom: 3 }}>Gói hiện tại</p>
                <p style={{ color: theme.primaryPale, fontSize: 18, fontWeight: 700 }}>{org.plan} Plan</p>
              </div>
              <Badge label="Active" />
            </div>
            {[{ l: 'Chu kỳ', v: 'Tháng' }, { l: 'Ngày gia hạn', v: '01/06/2025' }, { l: 'Phương thức', v: 'Chuyển khoản ngân hàng' }].map(r => (
              <div key={r.l} style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: `1px solid ${theme.border}` }}>
                <span style={{ color: theme.textMuted, fontSize: 13 }}>{r.l}</span>
                <span style={{ color: theme.text, fontSize: 13 }}>{r.v}</span>
              </div>
            ))}
            <button style={{ ...primaryBtn(), width: '100%', padding: '11px', marginTop: 16, fontSize: 13 } as React.CSSProperties}>Nâng cấp lên Enterprise</button>
          </div>
          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 14 }}>Mức sử dụng tháng này</p>
            {[{ l: 'AI Messages', c: 8400, m: 10000 }, { l: 'Leads', c: 284, m: 500 }, { l: 'Nhân viên', c: staff.length, m: 20 }].map(u => (
              <div key={u.l} style={{ marginBottom: 13 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 5 }}>
                  <span style={{ color: theme.textMuted, fontSize: 12 }}>{u.l}</span>
                  <span style={{ color: theme.text, fontSize: 12 }}>{u.c.toLocaleString()} / {u.m.toLocaleString()}</span>
                </div>
                <div style={{ height: 5, background: '#2a2f45', borderRadius: 99 }}>
                  <div style={{ width: `${Math.min(Math.round(u.c / u.m * 100), 100)}%`, height: '100%', background: u.c / u.m > 0.85 ? theme.red : theme.primary, borderRadius: 99 }} />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
