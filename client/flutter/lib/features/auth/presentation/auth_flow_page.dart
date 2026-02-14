import 'package:animations/animations.dart';
import 'package:arc/app/widgets/glass_scaffold.dart';
import 'package:arc/core/diagnostics/backend_status.dart';
import 'package:arc/core/config/app_config.dart';
import 'package:arc/features/auth/data/providers.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:arc/features/auth/presentation/auth_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class AuthFlowPage extends ConsumerWidget {
  const AuthFlowPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(authControllerProvider);
    final controller = ref.read(authControllerProvider.notifier);
    final config = ref.watch(appConfigProvider);
    final backend = ref.watch(backendProbeProvider);
    final platform = ref.watch(platformInfoProvider);

    return ArcGlassScaffold(
      hero: const ArcHeroPanel(),
      heroCollapsed: const ArcHeroCompact(),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _Header(view: state.view),
          const SizedBox(height: 12),
          _FlowSteps(view: state.view),
          if (state.error != null || state.info != null) ...[
            const SizedBox(height: 12),
            _StatusBanner(
              error: state.error,
              info: state.info,
              onClose: controller.clearMessages,
            ),
          ],
          const SizedBox(height: 12),
          Expanded(
            child: PageTransitionSwitcher(
              duration: const Duration(milliseconds: 340),
              reverse: false,
              transitionBuilder: (child, animation, secondaryAnimation) {
                return SharedAxisTransition(
                  animation: animation,
                  secondaryAnimation: secondaryAnimation,
                  transitionType: SharedAxisTransitionType.horizontal,
                  fillColor: Colors.transparent,
                  child: child,
                );
              },
              child: _buildView(state: state, controller: controller),
            ),
          ),
          const SizedBox(height: 12),
          _FooterHint(
            config: config,
            backend: backend,
            isWeb: platform.isWeb,
            onRecheck: () => ref.read(backendProbeProvider.notifier).checkNow(),
          ),
        ],
      ),
    );
  }

  Widget _buildView({
    required AuthState state,
    required AuthController controller,
  }) {
    return switch (state.view) {
      AuthView.bootstrapping => const _BootCard(key: ValueKey('view_boot')),
      AuthView.invite => _InviteCard(
        key: const ValueKey('view_invite'),
        loading: state.submitting,
        onSubmit: controller.submitInviteToken,
        onGoLogin: controller.goToLogin,
        inviteToken: state.inviteToken,
      ),
      AuthView.signup => _SignupCard(
        key: const ValueKey('view_signup'),
        loading: state.submitting,
        inviteOnly: state.inviteOnly,
        inviteToken: state.inviteToken,
        onSubmit:
            ({
              required inviteToken,
              required username,
              required email,
              required password,
              required rememberMe,
            }) {
              return controller.signupWithInvite(
                inviteToken: inviteToken,
                username: username,
                email: email,
                password: password,
                rememberMe: rememberMe,
              );
            },
        onGoLogin: controller.goToLogin,
        onGoInvite: controller.goToInvite,
      ),
      AuthView.login => _LoginCard(
        key: const ValueKey('view_login'),
        loading: state.submitting,
        inviteOnly: state.inviteOnly,
        onSubmit: ({username, email, required password, required rememberMe}) {
          return controller.login(
            username: username,
            email: email,
            password: password,
            rememberMe: rememberMe,
          );
        },
        onGoInvite: controller.goToInvite,
        onGoSignup: controller.goToSignup,
      ),
      AuthView.profile => _ProfileCard(
        key: const ValueKey('view_profile'),
        loading: state.submitting,
        user: state.user,
        onSubmit: controller.completeProfile,
      ),
      AuthView.authenticated => _AuthenticatedCard(
        key: const ValueKey('view_authenticated'),
        loading: state.submitting,
        user: state.user,
        session: state.session,
        latestInvite: state.latestInvite,
        onRefresh: controller.refreshUser,
        onLogout: () => controller.logout(all: false),
        onLogoutAll: () => controller.logout(all: true),
        onCreateInvite: ({int maxUses = 1, Duration? ttl}) {
          return controller.createInvite(maxUses: maxUses, ttl: ttl);
        },
      ),
    };
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.view});

  final AuthView view;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;

    final title = switch (view) {
      AuthView.bootstrapping => 'Preparing Secure Session',
      AuthView.invite => 'Enter Invite',
      AuthView.signup => 'Create Account',
      AuthView.login => 'Sign In',
      AuthView.profile => 'Complete Profile',
      AuthView.authenticated => 'Authenticated',
    };

    final subtitle = switch (view) {
      AuthView.bootstrapping =>
        'Loading persisted state and validating credentials.',
      AuthView.invite => 'Paste your invite token to unlock signup.',
      AuthView.signup => 'Create your Arc account with strong credentials.',
      AuthView.login => 'Sign in with username or email to continue.',
      AuthView.profile => 'Add display information for your identity.',
      AuthView.authenticated =>
        'Your session is active and ready for realtime.',
    };

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title, style: textTheme.headlineMedium),
        const SizedBox(height: 6),
        Text(subtitle, style: textTheme.bodyMedium),
      ],
    );
  }
}

class _FlowSteps extends StatelessWidget {
  const _FlowSteps({required this.view});

  final AuthView view;

  static const _labels = ['Invite', 'Signup', 'Login', 'Profile', 'Ready'];

  int get _activeIndex {
    return switch (view) {
      AuthView.bootstrapping => 0,
      AuthView.invite => 0,
      AuthView.signup => 1,
      AuthView.login => 2,
      AuthView.profile => 3,
      AuthView.authenticated => 4,
    };
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;

    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (var i = 0; i < _labels.length; i++)
          AnimatedContainer(
            duration: const Duration(milliseconds: 220),
            curve: Curves.easeOut,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(999),
              color: i <= _activeIndex
                  ? scheme.primary.withValues(alpha: 0.2)
                  : scheme.surface.withValues(alpha: 0.48),
              border: Border.all(
                color: i <= _activeIndex
                    ? scheme.primary.withValues(alpha: 0.42)
                    : scheme.outline.withValues(alpha: 0.3),
              ),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  i < _activeIndex
                      ? Icons.check_rounded
                      : i == _activeIndex
                      ? Icons.radio_button_checked_rounded
                      : Icons.radio_button_unchecked_rounded,
                  size: 16,
                ),
                const SizedBox(width: 6),
                Text(
                  _labels[i],
                  style: Theme.of(context).textTheme.labelMedium,
                ),
              ],
            ),
          ),
      ],
    );
  }
}

class _StatusBanner extends StatelessWidget {
  const _StatusBanner({
    required this.error,
    required this.info,
    required this.onClose,
  });

  final String? error;
  final String? info;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final isError = error != null;

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: isError
            ? scheme.error.withValues(alpha: 0.14)
            : scheme.primary.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: isError
              ? scheme.error.withValues(alpha: 0.72)
              : scheme.primary.withValues(alpha: 0.5),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 2),
            child: Icon(
              isError
                  ? Icons.error_outline_rounded
                  : Icons.info_outline_rounded,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              error ?? info ?? '',
              maxLines: 6,
              overflow: TextOverflow.fade,
            ),
          ),
          IconButton(
            onPressed: onClose,
            visualDensity: VisualDensity.compact,
            icon: const Icon(Icons.close_rounded),
          ),
        ],
      ),
    );
  }
}

class _FooterHint extends StatelessWidget {
  const _FooterHint({
    required this.config,
    required this.backend,
    required this.isWeb,
    required this.onRecheck,
  });

  final AppConfig config;
  final BackendStatusState backend;
  final bool isWeb;
  final VoidCallback onRecheck;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final statusColor = switch (backend.level) {
      BackendStatusLevel.online => const Color(0xFF1FA971),
      BackendStatusLevel.degraded => const Color(0xFFE39E2D),
      BackendStatusLevel.offline => const Color(0xFFD0342C),
      BackendStatusLevel.checking => scheme.primary,
    };
    final checkedAt = backend.checkedAt?.toLocal();
    final latency = backend.latency?.inMilliseconds;
    final effectiveCookie = isWeb && config.webCookieMode;
    final apiDisplay = (() {
      final uri = config.apiBaseUri;
      final hasPort = uri.hasPort;
      return hasPort ? '${uri.host}:${uri.port}' : uri.host;
    })();

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(14),
        color: scheme.surface.withValues(alpha: 0.52),
        border: Border.all(color: scheme.onSurface.withValues(alpha: 0.14)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 10,
                height: 10,
                decoration: BoxDecoration(
                  color: statusColor,
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: statusColor.withValues(alpha: 0.4),
                      blurRadius: 8,
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  backend.message ?? 'Checking server status...',
                  style: textTheme.bodySmall?.copyWith(
                    color: scheme.onSurface.withValues(alpha: 0.9),
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              TextButton(onPressed: onRecheck, child: const Text('Recheck')),
            ],
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 12,
            runSpacing: 6,
            children: [
              _MetaChip(label: 'Endpoint', value: apiDisplay),
              _MetaChip(
                label: 'Invite',
                value: config.inviteOnly ? 'On' : 'Off',
              ),
              _MetaChip(
                label: 'Cookie',
                value: effectiveCookie ? 'Web' : 'Token',
              ),
              if (latency != null && latency >= 0)
                _MetaChip(label: 'Latency', value: '${latency}ms'),
              if (checkedAt != null)
                _MetaChip(
                  label: 'Last check',
                  value:
                      '${checkedAt.hour.toString().padLeft(2, '0')}:${checkedAt.minute.toString().padLeft(2, '0')}:${checkedAt.second.toString().padLeft(2, '0')}',
                ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            'Arc Flutter Client • secure auth transport • realtime ready',
            style: textTheme.bodySmall,
          ),
        ],
      ),
    );
  }
}

class _MetaChip extends StatelessWidget {
  const _MetaChip({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;

    return Container(
      constraints: const BoxConstraints(maxWidth: 250),
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 7),
      decoration: BoxDecoration(
        color: scheme.surface.withValues(alpha: 0.62),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: scheme.onSurface.withValues(alpha: 0.12)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            '$label: ',
            style: textTheme.labelSmall?.copyWith(
              color: scheme.onSurface.withValues(alpha: 0.62),
              fontWeight: FontWeight.w600,
            ),
          ),
          Flexible(
            child: Text(
              value,
              overflow: TextOverflow.ellipsis,
              style: textTheme.labelSmall?.copyWith(
                color: scheme.onSurface.withValues(alpha: 0.92),
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _BootCard extends StatelessWidget {
  const _BootCard({super.key});

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        return SingleChildScrollView(
          key: const ValueKey('boot_content'),
          child: ConstrainedBox(
            constraints: BoxConstraints(minHeight: constraints.maxHeight),
            child: const Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  SizedBox(
                    height: 30,
                    width: 30,
                    child: CircularProgressIndicator(strokeWidth: 2.4),
                  ),
                  SizedBox(height: 14),
                  Text('Bootstrapping auth state...'),
                ],
              ),
            ),
          ),
        );
      },
    );
  }
}

class _InviteCard extends StatefulWidget {
  const _InviteCard({
    super.key,
    required this.loading,
    required this.onSubmit,
    required this.onGoLogin,
    required this.inviteToken,
  });

  final bool loading;
  final Future<void> Function(String token) onSubmit;
  final VoidCallback onGoLogin;
  final String inviteToken;

  @override
  State<_InviteCard> createState() => _InviteCardState();
}

class _InviteCardState extends State<_InviteCard> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _inviteController;

  @override
  void initState() {
    super.initState();
    _inviteController = TextEditingController(text: widget.inviteToken);
  }

  @override
  void dispose() {
    _inviteController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      autovalidateMode: AutovalidateMode.onUserInteraction,
      child: ListView(
        key: const ValueKey('invite_content'),
        children: [
          TextFormField(
            key: const ValueKey('field_invite_token'),
            controller: _inviteController,
            enabled: !widget.loading,
            textInputAction: TextInputAction.done,
            decoration: const InputDecoration(
              labelText: 'Invite Token',
              hintText: 'Paste invite token',
              helperText: 'Tokens are usually 40+ characters.',
              prefixIcon: Icon(Icons.key_rounded),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (v.isEmpty) {
                return 'Invite token is required.';
              }
              if (v.length < 20) {
                return 'Invite token looks too short.';
              }
              return null;
            },
            onFieldSubmitted: (_) => _submit(),
          ),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              key: const ValueKey('btn_continue_signup'),
              onPressed: widget.loading ? null : _submit,
              icon: widget.loading
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.arrow_forward_rounded),
              label: const Text('Continue to Signup'),
            ),
          ),
          const SizedBox(height: 10),
          TextButton(
            onPressed: widget.loading ? null : widget.onGoLogin,
            child: const Text('Already have an account? Sign in'),
          ),
        ],
      ),
    );
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) {
      return;
    }
    await widget.onSubmit(_inviteController.text);
  }
}

class _SignupCard extends StatefulWidget {
  const _SignupCard({
    super.key,
    required this.loading,
    required this.inviteOnly,
    required this.inviteToken,
    required this.onSubmit,
    required this.onGoLogin,
    required this.onGoInvite,
  });

  final bool loading;
  final bool inviteOnly;
  final String inviteToken;
  final Future<void> Function({
    required String inviteToken,
    required String? username,
    required String? email,
    required String password,
    required bool rememberMe,
  })
  onSubmit;
  final VoidCallback onGoLogin;
  final VoidCallback onGoInvite;

  @override
  State<_SignupCard> createState() => _SignupCardState();
}

class _SignupCardState extends State<_SignupCard> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _inviteController;
  final _usernameController = TextEditingController();
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  bool _rememberMe = false;
  bool _showPassword = false;

  @override
  void initState() {
    super.initState();
    _inviteController = TextEditingController(text: widget.inviteToken);
  }

  @override
  void dispose() {
    _inviteController.dispose();
    _usernameController.dispose();
    _emailController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      autovalidateMode: AutovalidateMode.onUserInteraction,
      child: ListView(
        key: const ValueKey('signup_content'),
        children: [
          if (widget.inviteOnly) ...[
            TextFormField(
              controller: _inviteController,
              enabled: !widget.loading,
              decoration: const InputDecoration(
                labelText: 'Invite Token',
                prefixIcon: Icon(Icons.key_rounded),
              ),
              validator: (value) {
                if ((value ?? '').trim().isEmpty) {
                  return 'Invite token is required.';
                }
                return null;
              },
            ),
            const SizedBox(height: 12),
          ],
          TextFormField(
            controller: _usernameController,
            enabled: !widget.loading,
            textInputAction: TextInputAction.next,
            decoration: const InputDecoration(
              labelText: 'Username',
              hintText: '3-32 characters',
              prefixIcon: Icon(Icons.alternate_email_rounded),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (_emailController.text.trim().isEmpty && v.isEmpty) {
                return 'Username or email is required.';
              }
              if (v.isNotEmpty && (v.length < 3 || v.length > 32)) {
                return 'Username must be 3-32 characters.';
              }
              return null;
            },
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: _emailController,
            enabled: !widget.loading,
            keyboardType: TextInputType.emailAddress,
            textInputAction: TextInputAction.next,
            decoration: const InputDecoration(
              labelText: 'Email (optional)',
              prefixIcon: Icon(Icons.mail_outline_rounded),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (_usernameController.text.trim().isEmpty && v.isEmpty) {
                return 'Username or email is required.';
              }
              if (v.isNotEmpty && !_isLikelyEmail(v)) {
                return 'Enter a valid email address.';
              }
              return null;
            },
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: _passwordController,
            enabled: !widget.loading,
            obscureText: !_showPassword,
            textInputAction: TextInputAction.done,
            decoration: InputDecoration(
              labelText: 'Password',
              hintText: 'At least 8 characters',
              prefixIcon: const Icon(Icons.lock_outline_rounded),
              suffixIcon: IconButton(
                onPressed: widget.loading
                    ? null
                    : () {
                        setState(() {
                          _showPassword = !_showPassword;
                        });
                      },
                icon: Icon(
                  _showPassword ? Icons.visibility_off : Icons.visibility,
                ),
              ),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (v.length < 8) {
                return 'Password must be at least 8 characters.';
              }
              return null;
            },
            onFieldSubmitted: (_) => _submit(),
          ),
          const SizedBox(height: 10),
          CheckboxListTile(
            value: _rememberMe,
            contentPadding: EdgeInsets.zero,
            title: const Text('Remember this device'),
            onChanged: widget.loading
                ? null
                : (value) {
                    setState(() {
                      _rememberMe = value ?? false;
                    });
                  },
          ),
          const SizedBox(height: 8),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              key: const ValueKey('btn_signup'),
              onPressed: widget.loading ? null : _submit,
              icon: widget.loading
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.person_add_alt_1_rounded),
              label: const Text('Create Account'),
            ),
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 10,
            children: [
              TextButton(
                onPressed: widget.loading ? null : widget.onGoLogin,
                child: const Text('Already have account? Login'),
              ),
              if (widget.inviteOnly)
                TextButton(
                  onPressed: widget.loading ? null : widget.onGoInvite,
                  child: const Text('Use another invite'),
                ),
            ],
          ),
        ],
      ),
    );
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) {
      return;
    }

    await widget.onSubmit(
      inviteToken: _inviteController.text.trim(),
      username: _trimOrNull(_usernameController.text),
      email: _trimOrNull(_emailController.text),
      password: _passwordController.text,
      rememberMe: _rememberMe,
    );
  }
}

enum _LoginIdentityMode { username, email }

class _LoginCard extends StatefulWidget {
  const _LoginCard({
    super.key,
    required this.loading,
    required this.inviteOnly,
    required this.onSubmit,
    required this.onGoInvite,
    required this.onGoSignup,
  });

  final bool loading;
  final bool inviteOnly;
  final Future<void> Function({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  })
  onSubmit;
  final VoidCallback onGoInvite;
  final VoidCallback onGoSignup;

  @override
  State<_LoginCard> createState() => _LoginCardState();
}

class _LoginCardState extends State<_LoginCard> {
  final _formKey = GlobalKey<FormState>();
  final _identifierController = TextEditingController();
  final _passwordController = TextEditingController();
  _LoginIdentityMode _mode = _LoginIdentityMode.username;
  bool _rememberMe = false;
  bool _showPassword = false;

  @override
  void dispose() {
    _identifierController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      autovalidateMode: AutovalidateMode.onUserInteraction,
      child: ListView(
        key: const ValueKey('login_content'),
        children: [
          SegmentedButton<_LoginIdentityMode>(
            segments: const [
              ButtonSegment(
                value: _LoginIdentityMode.username,
                label: Text('Username'),
              ),
              ButtonSegment(
                value: _LoginIdentityMode.email,
                label: Text('Email'),
              ),
            ],
            selected: {_mode},
            onSelectionChanged: widget.loading
                ? null
                : (next) {
                    setState(() {
                      _mode = next.first;
                    });
                  },
          ),
          const SizedBox(height: 14),
          TextFormField(
            controller: _identifierController,
            enabled: !widget.loading,
            keyboardType: _mode == _LoginIdentityMode.email
                ? TextInputType.emailAddress
                : TextInputType.text,
            textInputAction: TextInputAction.next,
            decoration: InputDecoration(
              labelText: _mode == _LoginIdentityMode.email
                  ? 'Email'
                  : 'Username',
              prefixIcon: Icon(
                _mode == _LoginIdentityMode.email
                    ? Icons.mail_outline_rounded
                    : Icons.alternate_email_rounded,
              ),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (v.isEmpty) {
                return _mode == _LoginIdentityMode.email
                    ? 'Email is required.'
                    : 'Username is required.';
              }
              if (_mode == _LoginIdentityMode.email && !_isLikelyEmail(v)) {
                return 'Enter a valid email address.';
              }
              return null;
            },
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: _passwordController,
            enabled: !widget.loading,
            obscureText: !_showPassword,
            textInputAction: TextInputAction.done,
            decoration: InputDecoration(
              labelText: 'Password',
              prefixIcon: const Icon(Icons.lock_outline_rounded),
              suffixIcon: IconButton(
                onPressed: widget.loading
                    ? null
                    : () {
                        setState(() {
                          _showPassword = !_showPassword;
                        });
                      },
                icon: Icon(
                  _showPassword ? Icons.visibility_off : Icons.visibility,
                ),
              ),
            ),
            validator: (value) {
              if ((value ?? '').trim().isEmpty) {
                return 'Password is required.';
              }
              return null;
            },
            onFieldSubmitted: (_) => _submit(),
          ),
          const SizedBox(height: 10),
          CheckboxListTile(
            value: _rememberMe,
            contentPadding: EdgeInsets.zero,
            title: const Text('Remember this device'),
            onChanged: widget.loading
                ? null
                : (value) {
                    setState(() {
                      _rememberMe = value ?? false;
                    });
                  },
          ),
          const SizedBox(height: 8),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              key: const ValueKey('btn_login'),
              onPressed: widget.loading ? null : _submit,
              icon: widget.loading
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.login_rounded),
              label: const Text('Sign In'),
            ),
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 10,
            children: [
              TextButton(
                onPressed: widget.loading ? null : widget.onGoSignup,
                child: const Text('Need account? Signup'),
              ),
              if (widget.inviteOnly)
                TextButton(
                  onPressed: widget.loading ? null : widget.onGoInvite,
                  child: const Text('Back to invite'),
                ),
            ],
          ),
        ],
      ),
    );
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) {
      return;
    }

    final identifier = _identifierController.text.trim();

    await widget.onSubmit(
      username: _mode == _LoginIdentityMode.username ? identifier : null,
      email: _mode == _LoginIdentityMode.email ? identifier : null,
      password: _passwordController.text,
      rememberMe: _rememberMe,
    );
  }
}

class _ProfileCard extends StatefulWidget {
  const _ProfileCard({
    super.key,
    required this.loading,
    required this.user,
    required this.onSubmit,
  });

  final bool loading;
  final ArcUser? user;
  final Future<void> Function(ProfileDraft draft) onSubmit;

  @override
  State<_ProfileCard> createState() => _ProfileCardState();
}

class _ProfileCardState extends State<_ProfileCard> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _displayName;
  late final TextEditingController _username;
  late final TextEditingController _bio;

  @override
  void initState() {
    super.initState();
    _displayName = TextEditingController(text: widget.user?.displayName ?? '');
    _username = TextEditingController(text: widget.user?.username ?? '');
    _bio = TextEditingController(text: widget.user?.bio ?? '');
  }

  @override
  void dispose() {
    _displayName.dispose();
    _username.dispose();
    _bio.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      autovalidateMode: AutovalidateMode.onUserInteraction,
      child: ListView(
        key: const ValueKey('profile_content'),
        children: [
          TextFormField(
            controller: _displayName,
            enabled: !widget.loading,
            textInputAction: TextInputAction.next,
            decoration: const InputDecoration(
              labelText: 'Display Name',
              hintText: 'Shown to other users',
              prefixIcon: Icon(Icons.badge_outlined),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (v.isEmpty) {
                return 'Display name is required.';
              }
              if (v.length > 80) {
                return 'Display name must be 80 characters or less.';
              }
              return null;
            },
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: _username,
            enabled: !widget.loading,
            textInputAction: TextInputAction.next,
            decoration: const InputDecoration(
              labelText: 'Username (optional)',
              prefixIcon: Icon(Icons.alternate_email_rounded),
            ),
            validator: (value) {
              final v = (value ?? '').trim();
              if (v.isNotEmpty && (v.length < 3 || v.length > 32)) {
                return 'Username must be 3-32 characters.';
              }
              return null;
            },
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: _bio,
            enabled: !widget.loading,
            maxLines: 4,
            maxLength: 512,
            textInputAction: TextInputAction.done,
            decoration: const InputDecoration(
              labelText: 'Bio (optional)',
              alignLabelWithHint: true,
            ),
            onFieldSubmitted: (_) => _submit(),
          ),
          const SizedBox(height: 8),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              key: const ValueKey('btn_profile_continue'),
              onPressed: widget.loading ? null : _submit,
              icon: widget.loading
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.check_circle_outline_rounded),
              label: const Text('Continue'),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) {
      return;
    }

    await widget.onSubmit(
      ProfileDraft(
        displayName: _displayName.text,
        username: _trimOrNull(_username.text),
        bio: _trimOrNull(_bio.text),
      ),
    );
  }
}

class _AuthenticatedCard extends StatelessWidget {
  const _AuthenticatedCard({
    super.key,
    required this.loading,
    required this.user,
    required this.session,
    required this.latestInvite,
    required this.onRefresh,
    required this.onLogout,
    required this.onLogoutAll,
    required this.onCreateInvite,
  });

  final bool loading;
  final ArcUser? user;
  final ArcSession? session;
  final ArcInvite? latestInvite;
  final Future<void> Function() onRefresh;
  final Future<void> Function() onLogout;
  final Future<void> Function() onLogoutAll;
  final Future<void> Function({required int maxUses, Duration? ttl})
  onCreateInvite;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final userName = user?.displayName?.trim().isNotEmpty == true
        ? user!.displayName!.trim()
        : user?.username?.trim().isNotEmpty == true
        ? user!.username!.trim()
        : user?.email?.trim().isNotEmpty == true
        ? user!.email!.trim()
        : 'Arc User';

    return ListView(
      key: const ValueKey('authed_content'),
      children: [
        Text('Welcome, $userName', style: textTheme.titleLarge),
        const SizedBox(height: 4),
        Text(
          'Your session is active. You can rotate credentials, fetch profile state, and issue onboarding invites.',
          style: textTheme.bodySmall,
        ),
        const SizedBox(height: 12),
        _DataTile(label: 'User ID', value: user?.id ?? '-'),
        _DataTile(label: 'Session ID', value: session?.sessionId ?? '-'),
        _DataTile(
          label: 'Access Expires',
          value: session?.accessExpiresAt.toLocal().toString() ?? '-',
        ),
        _DataTile(
          label: 'Refresh Expires',
          value: session?.refreshExpiresAt.toLocal().toString() ?? '-',
        ),
        const SizedBox(height: 6),
        _InviteComposer(
          loading: loading,
          latestInvite: latestInvite,
          onCreateInvite: onCreateInvite,
        ),
        const SizedBox(height: 14),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton.icon(
            onPressed: loading ? null : onRefresh,
            icon: const Icon(Icons.sync_rounded),
            label: const Text('Refresh Session + Profile'),
          ),
        ),
        const SizedBox(height: 10),
        SizedBox(
          width: double.infinity,
          child: OutlinedButton.icon(
            onPressed: loading ? null : onLogout,
            icon: const Icon(Icons.logout_rounded),
            label: const Text('Logout Current Session'),
          ),
        ),
        const SizedBox(height: 10),
        SizedBox(
          width: double.infinity,
          child: OutlinedButton.icon(
            onPressed: loading ? null : onLogoutAll,
            icon: const Icon(Icons.gpp_maybe_rounded),
            label: const Text('Logout All Sessions'),
          ),
        ),
      ],
    );
  }
}

class _InviteComposer extends StatefulWidget {
  const _InviteComposer({
    required this.loading,
    required this.latestInvite,
    required this.onCreateInvite,
  });

  final bool loading;
  final ArcInvite? latestInvite;
  final Future<void> Function({required int maxUses, Duration? ttl})
  onCreateInvite;

  @override
  State<_InviteComposer> createState() => _InviteComposerState();
}

class _InviteComposerState extends State<_InviteComposer> {
  int _maxUses = 1;
  int _ttlDays = 7;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final invite = widget.latestInvite;

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: scheme.surface.withValues(alpha: 0.6),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: scheme.onSurface.withValues(alpha: 0.14)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Invite Generator', style: textTheme.titleMedium),
          const SizedBox(height: 4),
          Text(
            'Create tokenized invite links for onboarding.',
            style: textTheme.bodySmall,
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              Expanded(
                child: DropdownButtonFormField<int>(
                  initialValue: _maxUses,
                  decoration: const InputDecoration(
                    labelText: 'Max Uses',
                    prefixIcon: Icon(Icons.groups_2_rounded),
                  ),
                  items: const [1, 2, 3, 5, 10]
                      .map((v) => DropdownMenuItem(value: v, child: Text('$v')))
                      .toList(),
                  onChanged: widget.loading
                      ? null
                      : (v) {
                          if (v == null) {
                            return;
                          }
                          setState(() {
                            _maxUses = v;
                          });
                        },
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: DropdownButtonFormField<int>(
                  initialValue: _ttlDays,
                  decoration: const InputDecoration(
                    labelText: 'TTL',
                    prefixIcon: Icon(Icons.timer_outlined),
                  ),
                  items: const [1, 3, 7, 14, 30]
                      .map(
                        (v) => DropdownMenuItem(
                          value: v,
                          child: Text(v == 1 ? '1 day' : '$v days'),
                        ),
                      )
                      .toList(),
                  onChanged: widget.loading
                      ? null
                      : (v) {
                          if (v == null) {
                            return;
                          }
                          setState(() {
                            _ttlDays = v;
                          });
                        },
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              onPressed: widget.loading
                  ? null
                  : () {
                      widget.onCreateInvite(
                        maxUses: _maxUses,
                        ttl: Duration(days: _ttlDays),
                      );
                    },
              icon: const Icon(Icons.key_rounded),
              label: const Text('Create Invite Token'),
            ),
          ),
          if (invite != null) ...[
            const SizedBox(height: 12),
            _DataTile(label: 'Invite ID', value: invite.inviteId),
            _DataTile(label: 'Token', value: invite.inviteToken),
            _DataTile(
              label: 'Expires',
              value: invite.expiresAt.toLocal().toString(),
            ),
            Align(
              alignment: Alignment.centerRight,
              child: OutlinedButton.icon(
                onPressed: () async {
                  await Clipboard.setData(
                    ClipboardData(text: invite.inviteToken),
                  );
                  if (!context.mounted) {
                    return;
                  }
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Invite token copied')),
                  );
                },
                icon: const Icon(Icons.copy_rounded),
                label: const Text('Copy Token'),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _DataTile extends StatelessWidget {
  const _DataTile({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;

    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: scheme.surface.withValues(alpha: 0.56),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: scheme.outline.withValues(alpha: 0.32)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(width: 120, child: Text(label, style: textTheme.labelLarge)),
          Expanded(
            child: SelectableText(
              value,
              style: textTheme.bodyMedium,
              maxLines: 2,
            ),
          ),
        ],
      ),
    );
  }
}

String? _trimOrNull(String value) {
  final trimmed = value.trim();
  if (trimmed.isEmpty) {
    return null;
  }
  return trimmed;
}

bool _isLikelyEmail(String value) {
  final v = value.trim();
  final at = v.indexOf('@');
  if (at <= 0 || at == v.length - 1) {
    return false;
  }
  return v.indexOf('.', at) > at + 1;
}
