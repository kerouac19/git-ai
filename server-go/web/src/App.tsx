import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import Layout from "./components/Layout";
import Login from "./routes/Login";
import Me from "./routes/Me";
import DeviceFlow from "./routes/DeviceFlow";
import DeviceResult from "./routes/DeviceResult";

const Dashboard = lazy(() => import("./routes/Dashboard"));

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/oauth/device" element={<DeviceFlow />} />
      <Route path="/oauth/device/result" element={<DeviceResult />} />
      
      {/* Authenticated routes with Sidebar Layout */}
      <Route
        path="/me"
        element={
          <Layout>
            <Me />
          </Layout>
        }
      />
      <Route
        path="/dashboard"
        element={
          <Layout>
            <Suspense fallback={<div className="page-main"><p className="muted">加载中…</p></div>}>
              <Dashboard />
            </Suspense>
          </Layout>
        }
      />

      <Route path="/admin/activity" element={<Navigate to="/dashboard" replace />} />
      <Route path="*" element={<Navigate to="/me" replace />} />
    </Routes>
  );
}

