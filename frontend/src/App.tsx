import { Navigate, Route, Routes } from "react-router-dom";
import Checkout from "./components/Checkout";
import EventPage from "./components/EventPage";
import Home from "./components/Home";
import Layout from "./components/Layout";
import Login from "./components/Login";
import Manage from "./components/Manage";
import Tickets from "./components/Tickets";

export function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<Login />} />
        <Route path="/events/:id" element={<EventPage />} />
        <Route path="/checkout/:reservationId" element={<Checkout />} />
        <Route path="/tickets" element={<Tickets />} />
        <Route path="/manage" element={<Manage />} />
        <Route path="*" element={<Navigate to="/" />} />
      </Routes>
    </Layout>
  );
}
