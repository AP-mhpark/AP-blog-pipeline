import { Routes, Route } from "react-router";
import PostListPage from "./pages/PostListPage";
import NewPostPage from "./pages/NewPostPage";
import PostDetailPage from "./pages/PostDetailPage";

export default function App() {
  return (
    <div style={{ maxWidth: 960, margin: "0 auto", padding: "1.5rem" }}>
      <Routes>
        <Route path="/" element={<PostListPage />} />
        <Route path="/posts/new" element={<NewPostPage />} />
        <Route path="/posts/:id" element={<PostDetailPage />} />
      </Routes>
    </div>
  );
}
