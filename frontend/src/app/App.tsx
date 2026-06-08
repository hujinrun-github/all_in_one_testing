import {
  DashboardOutlined,
  FileTextOutlined,
  NodeIndexOutlined,
  PlayCircleOutlined,
  ProfileOutlined,
} from '@ant-design/icons';
import { Layout, Menu, Typography } from 'antd';

const navItems = [
  { key: 'dashboard', icon: <DashboardOutlined />, label: <a href="#dashboard">Dashboard</a> },
  { key: 'targets', icon: <NodeIndexOutlined />, label: <a href="#targets">Targets & Agents</a> },
  { key: 'scenarios', icon: <ProfileOutlined />, label: <a href="#scenarios">Scenarios</a> },
  { key: 'runs', icon: <PlayCircleOutlined />, label: <a href="#runs">Runs</a> },
  { key: 'reports', icon: <FileTextOutlined />, label: <a href="#reports">Reports</a> },
];

export default function App() {
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Layout.Sider breakpoint="lg" collapsedWidth={0}>
        <nav aria-label="Project workspace">
          <Menu theme="dark" mode="inline" selectedKeys={['dashboard']} items={navItems} />
        </nav>
      </Layout.Sider>
      <Layout>
        <Layout.Header style={{ background: '#ffffff', borderBottom: '1px solid #e5e7eb' }}>
          <Typography.Title level={1} style={{ fontSize: 22, margin: 0 }}>
            All-in-One Testing
          </Typography.Title>
        </Layout.Header>
        <Layout.Content style={{ padding: 24 }}>
          <Typography.Title level={2} style={{ fontSize: 18 }}>
            Dashboard
          </Typography.Title>
        </Layout.Content>
      </Layout>
    </Layout>
  );
}
