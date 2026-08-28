import { forwardRef } from "react";
import { Link } from "react-router";
import type { LinkProps } from "react-router";

/**
 * A thin wrapper around React Router's `<Link>` that opts into
 * the router's built-in view transition support.
 */
const ViewTransitionLink = forwardRef<
  HTMLAnchorElement,
  LinkProps & React.AnchorHTMLAttributes<HTMLAnchorElement>
>(function ViewTransitionLink({ to, replace, state, onClick, children, ...rest }, ref) {
  return (
    <Link
      ref={ref}
      to={to}
      replace={replace}
      state={state}
      onClick={onClick}
      viewTransition
      {...rest}
    >
      {children}
    </Link>
  );
});

export default ViewTransitionLink;
