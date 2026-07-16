import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "CouchPilot Field Guide — 手柄键位文档",
  description: "CouchPilot 的全局键位、App 专属映射、震动反馈与安全规则。",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
