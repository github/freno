#
# This is a pt-archiver plugin that checks freno for throttling.
# Base on pt-archiver extensions, see https://www.percona.com/doc/percona-toolkit/2.2/pt-archiver.html
#
# You will want to put this file in your perl search path (@INC). Find that value via:
#   perl -e 'print "@INC"'
# A reasonable path would be /usr/share/perl5/FrenoThrottler.pm
#
# You will need to edit this file to let pt-archiver know where freno is located on your system, and what cluster name to use.
# More information in the `new` function.

package FrenoThrottler;

use LWP::Simple;
use Time::HiRes qw(time);

our $freno_url = "";
our $check_interval_seconds = 0.05;
our $last_check_time = 0;

sub throttle {
  my $time_now = time;
  if ($time_now - $last_check_time < $check_interval_seconds) {
    return 1;
  }
  $last_check_time = $time_now;
  # Consult freno, only proceed on HTTP OK (2XX)
  while (! head($freno_url)) {
    select(undef, undef, undef, 0.25); # sleep
  }
  return 1;
}

sub new {
  my ( $class, %args ) = @_;

  $freno_url = "TODO: Setup your freno URL here";

  # As example, you may read URL or cluster hint from your database:
  #
  #  my $dbh = $args{"dbh"};
  #  my ($cluster) = $dbh->selectrow_array("select cluster_name from meta.cluster limit 1");
  #  if ($cluster eq "" || not defined $cluster) {
  #    die "Cannot find cluster";
  #  }
  #  $freno_url = "http://my.freno.com:9777/check/pt-archiver/mysql/$cluster";

  return bless(\%args, $class);
}

sub before_begin {}

sub before_bulk_delete {
  return throttle()
}

sub before_bulk_insert {
  return throttle()
}

sub before_delete {
  return throttle()
}

sub before_insert {
  return throttle()
}

sub is_archivable {}

sub custom_sth    {} # Take no action
sub after_finish  {} # Take no action

1;
