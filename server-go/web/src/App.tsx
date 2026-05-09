import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import Login from "./routes/Login";
import Me from "./routes/Me";
import DeviceFlow from "./routes/DeviceFlow";
import DeviceResult from "./routes/DeviceResult";

const Dashboard = lazy(() => import("./routes/Dashboard"));

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/me" element={<Me />} />
      <Route path="/oauth/device" element={<DeviceFlow />} />
      <Route path="/oauth/device/result" element={<DeviceResult />} />
      <Route
        path="/dashboard"
        element={
          <Suspense fallback={<div style={{ padding: 24 }}>Loading…</div>}>
            <Dashboard />
          </Suspense>
        }
      />
      <Route path="/admin/activity" element={<Navigate to="/dashboard" replace />} />
      <Route path="*" element={<Navigate to="/me" replace />} />
    </Routes>
  );
}
