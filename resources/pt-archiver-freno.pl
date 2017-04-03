package Freno::Ptarchiver;

use LWP::Simple;

sub new {
   my ( $class, %args ) = @_;
   return bless(\%args, $class);
}

sub before_begin {}

sub is_archivable {}

sub before_delete {} # Take no action

sub before_bulk_delete {
  while(! head($freno_url)) {
    select(undef, undef, undef, 0.25); # sleep
  }
  return 1;
}

sub before_bulk_insert {
  while(! head($freno_url)) {
    select(undef, undef, undef, 0.25); # sleep
  }
  return 1;
}

sub before_insert {} # Take no action
sub custom_sth    {} # Take no action
sub after_finish  {} # Take no action

1;
