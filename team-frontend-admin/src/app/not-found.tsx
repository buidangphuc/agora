import { Result } from "@/components/ui/Result";

export default function NotFound() {
  return (
    <main className="mx-auto w-full max-w-page px-4 py-10">
      <Result
        status="404"
        title="Không tìm thấy trang"
        subTitle="Trang bạn tìm không tồn tại trong khu vực quản trị."
      />
    </main>
  );
}
