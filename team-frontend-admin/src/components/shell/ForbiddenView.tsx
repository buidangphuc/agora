import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";
import { logoutAction } from "@/features/auth/actions";

export const FORBIDDEN_TITLE = "Không có quyền";

export function ForbiddenView() {
  return (
    <main className="mx-auto w-full max-w-page px-4 py-10">
      <Result
        status="403"
        title={FORBIDDEN_TITLE}
        subTitle="Tài khoản này chưa được cấp vai trò quản trị sàn. Hãy đăng nhập bằng tài khoản quản trị khác."
        extra={
          <form action={logoutAction}>
            <Button type="submit">Đăng xuất</Button>
          </form>
        }
      />
    </main>
  );
}
